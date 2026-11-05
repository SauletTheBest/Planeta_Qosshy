#!/usr/bin/env bash
# Block B end-to-end: admin dashboard, orders (ship/deliver/cancel), refunds through the
# test bank, late payments, users (roles/blocking), CSRF guard, migration 000007.
# Throw-away container pq-tb only.
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
# Work folder for binaries, cookies, logs. NOT inside the repo: its path has a space and
# Cyrillic letters ("Рабочий стол"), which break unquoted arguments and curl.exe.
RUN="${RUN:-/tmp/planeta-e2e}"
S=http://127.0.0.1:8080; K=http://127.0.0.1:8081
KEY=$(openssl rand -hex 24); SECRET=$(openssl rand -hex 24); CS=$(openssl rand -base64 32)
pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "PASS  $1 ($2)"; pass=$((pass+1)); else echo "FAIL  $1 (got '$2', want '$3')"; fail=$((fail+1)); fi; }
q() { docker exec pq-tb psql -U postgres -d planeta_qosshy -Atc "$1"; }
wait_for() { for i in $(seq 1 "${3:-20}"); do [ "$(q "$1")" = "$2" ] && return 0; sleep 1; done; return 1; }
cx() { local ip=$1; shift; curl -s -H "X-Forwarded-For: $ip" "$@"; }
# -c: keep the new cookie (flash messages ride in it)
B="-b $RUN/boss.ck -c $RUN/boss.ck"
A="-b $RUN/alice.ck"
post() { curl -s -o /dev/null -w '%{http_code}' "$@"; }           # status code of a POST
page() { curl -s "$@"; }
paycard() { curl -s -o /dev/null -w '%{redirect_url}' --data-urlencode "card_number=$2" -d 'expiry=12/30&cvv=123&holder=IVAN' "$1"; }
ev() { echo "{\"id\":\"evt_test\",\"type\":\"$1\",\"payment_id\":\"$2\",\"merchant_reference\":\"$3\",\"amount_tiyn\":$4,\"currency\":\"KZT\",\"card_last4\":\"4242\"}"; }
signed() {
  local sig; sig=$(printf '%s.%s' "$2" "$1" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed 's/^.* //')
  curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -H "Qosshy-Signature: t=$2,v1=$sig" -d "$1" $S/payments/webhook
}
buy() { curl -s -o /dev/null -w '%{redirect_url}' $A -d 'size=M' $S/payment/$P; }   # → bank page URL
last_order() { q "SELECT max(id) FROM orders"; }
variant() { q "SELECT stock FROM clothes_variants WHERE clothes_id=$P AND size='M'"; }
events() { q "SELECT string_agg(action, ',' ORDER BY id) FROM order_events WHERE order_id=$1"; }
flash_on() { page $B "$1" | grep -c "$2"; }

start_bank() {
  mkdir -p "$RUN/bank"; cd "$RUN/bank"
  PORT=8081 PUBLIC_URL=$K API_KEY=$KEY WEBHOOK_SECRET=$SECRET MERCHANT_WEBHOOK_URL=$S/payments/webhook MERCHANT_SITE_URL=$S \
    "$RUN/payments.exe" > "$RUN/bankB.out" 2>&1 & BANK=$!
  cd "$RUN"
}
start_shop() {  # $1 = PAYMENT_API_URL
  cd "$REPO"
  env "DATABASE_URL=host=127.0.0.1 port=55442 user=postgres password=test dbname=planeta_qosshy sslmode=disable" \
    COOKIE_SECRET=$CS PAYMENT_API_URL=$1 PAYMENT_API_KEY=$KEY PAYMENT_WEBHOOK_SECRET=$SECRET \
    PUBLIC_URL=$S SMTP_HOST= SMTP_USER= TRUSTED_PROXIES=127.0.0.1 \
    ORDER_RESERVATION_TTL=20s ORDER_EXPIRY_GRACE=2s ORDER_EXPIRY_INTERVAL=2s \
    "$RUN/app.exe" >> "$RUN/shopB.out" 2>&1 & APP=$!
  cd "$RUN"
  for i in $(seq 1 40); do curl -s -o /dev/null $S/ && return 0; sleep 0.5; done; return 1
}
stop_shop() { kill $APP 2>/dev/null; wait $APP 2>/dev/null; taskkill //F //IM app.exe >/dev/null 2>&1; sleep 0.5; }

: > "$RUN/shopB.out"
docker run -d --rm --name pq-tb -e POSTGRES_PASSWORD=test -e POSTGRES_DB=planeta_qosshy -p 127.0.0.1:55442:5432 --memory 256m postgres:16-alpine >/dev/null
for i in $(seq 1 30); do docker exec pq-tb pg_isready -U postgres -d planeta_qosshy >/dev/null 2>&1 && break; sleep 1; done; sleep 2
start_bank
start_shop $K || echo "shop did not start"
for i in $(seq 1 20); do curl -s -o /dev/null $K/pay/x && break; sleep 0.5; done

echo "== 0. setup + migration 000007"
for u in boss alice bob; do cx 10.1.0.1$((n=n+1)) -o /dev/null -d "username=$u&email=$u@example.com&password=supersecret1" $S/auth/register; done
q "UPDATE users SET role='admin' WHERE email='boss@example.com'" >/dev/null
for u in boss alice bob; do cx 10.1.1.1$((m=m+1)) -o /dev/null -c "$RUN/$u.ck" -d "email=$u@example.com&password=supersecret1" $S/auth/login; done
post $B --data-urlencode "size=M:5" --data-urlencode "price=10000" -d 'title=Admin hoodie&brand=Q&category=Худи&color=Red&image_url=&description=x' $S/admin/clothes >/dev/null
P=$(q "SELECT id FROM clothes WHERE title='Admin hoodie'")
check "schema at version 7 or later"        "$(q "SELECT version >= 7 AND NOT dirty FROM schema_migrations")" t
check "new user gets created_at"            "$(q "SELECT count(*) FROM users WHERE created_at IS NOT NULL")" 3
check "DB refuses an unknown role"          "$(q "UPDATE users SET role='superadmin' WHERE email='bob@example.com'" 2>&1 | grep -c users_role_check)" 1
check "admin pages closed to customers"     "$(post $A $S/admin/orders) $(post $A -d role=admin $S/admin/users/3/role)" "403 403"

echo "== 1. CSRF: forms from another site are refused"
r=$(curl -s $A -w '\n%{http_code}' -H 'Sec-Fetch-Site: cross-site' -d 'username=alice&email=alice@example.com' $S/profile/edit)
check "Sec-Fetch-Site: cross-site → 403"    "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'с другого сайта')" "403 1"
check "foreign Origin → 403"                "$(post $A -H 'Origin: http://evil.example' -d 'username=alice&email=alice@example.com' $S/profile/edit)" 403
check "same-origin → allowed"               "$(post $A -H 'Sec-Fetch-Site: same-origin' -d 'username=alice&email=alice@example.com' $S/profile/edit)" 303
check "our own Origin → allowed"            "$(post $A -H "Origin: $S" -d 'username=alice&email=alice@example.com' $S/profile/edit)" 303
check "cross-site GET is fine (links)"      "$(curl -s -o /dev/null -w '%{http_code}' -H 'Sec-Fetch-Site: cross-site' $S/clothes/)" 200
check "admin action from evil site → 403"   "$(post $B -H 'Sec-Fetch-Site: cross-site' -d role=admin $S/admin/users/3/role) $(q "SELECT role FROM users WHERE email='alice@example.com'")" "403 user"

echo "== 2. paid order: ship and deliver"
U1=$(buy); O1=$(last_order); paycard "$U1" "4242 4242 4242 4242" >/dev/null
wait_for "SELECT status FROM orders WHERE id=$O1" paid
check "paid, history says so"               "$(events $O1) | $(q "SELECT note FROM order_events WHERE order_id=$O1")" "paid | карта •••• 4242"
dash=$(page $B $S/admin/)
check "dashboard: revenue today + month"    "$(echo "$dash" | grep -c 'stat-number">10 000 ₸')" 2
check "dashboard: 1 to ship"                "$(echo "$dash" | grep -A1 'stat-number">1<' | grep -c 'Ждут отправки')" 1
list=$(page $B "$S/admin/orders?status=paid")
check "orders list: tab + row"              "$(echo "$list" | grep -c "Оплачен <b>1</b>") $(echo "$list" | grep -c "href=\"/admin/orders/$O1\"")" "1 1"
check "search by #number"                   "$(page $B "$S/admin/orders?q=%23$O1" | grep -c "href=\"/admin/orders/$O1\"")" 1
check "search by email part"                "$(page $B "$S/admin/orders?q=alice@" | grep -c "href=\"/admin/orders/$O1\"")" 1
check "'%' is a letter, not a wildcard"     "$(page $B "$S/admin/orders?q=%25" | grep -c 'Заказов не найдено')" 1
check "paid → pending is refused"           "$(post $B -d to=pending $S/admin/orders/$O1/status) $(q "SELECT status FROM orders WHERE id=$O1")" "303 paid"
check "  ...with a message"                 "$(flash_on $S/admin/orders/$O1 'переход статуса недоступен')" 1
post $B -d to=shipped $S/admin/orders/$O1/status >/dev/null
check "shipped"                             "$(q "SELECT status FROM orders WHERE id=$O1")" shipped
check "  ...flash shown once"               "$(flash_on $S/admin/orders/$O1 'отправлен') $(flash_on $S/admin/orders/$O1 'alert-success')" "1 0"
check "shipped can't be cancelled"          "$(post $B -d to=cancelled -d reason=x $S/admin/orders/$O1/status) $(q "SELECT status FROM orders WHERE id=$O1")" "303 shipped"
post $B -d to=delivered $S/admin/orders/$O1/status >/dev/null
check "delivered, history with the admin"   "$(events $O1) $(q "SELECT u.username FROM order_events e JOIN users u ON u.id=e.actor_id WHERE e.order_id=$O1 AND action='delivered'")" "paid,shipped,delivered boss"
check "final status: no buttons"            "$(page $B $S/admin/orders/$O1 | grep -c 'окончательный')" 1

echo "== 3. admin cancels a PAID order → money back through the bank"
U2=$(buy); O2=$(last_order); paycard "$U2" "4242 4242 4242 4242" >/dev/null
wait_for "SELECT status FROM orders WHERE id=$O2" paid
before=$(variant)
check "cancel needs a reason"               "$(post $B -d to=cancelled $S/admin/orders/$O2/status) $(q "SELECT status FROM orders WHERE id=$O2")" "303 paid"
printf 'нет на складе' > "$RUN/reason.txt"   # Cyrillic via a file: curl.exe args are not UTF-8 on Windows
post $B -d to=cancelled --data-urlencode "reason@$(cygpath -m "$RUN/reason.txt")" $S/admin/orders/$O2/status >/dev/null
check "order cancelled, stock back"         "$(q "SELECT status FROM orders WHERE id=$O2") $(( $(variant) - before ))" "cancelled 1"
check "payment refunded"                    "$(q "SELECT status||' '||(refunded_at IS NOT NULL)||' '||(refund_needed_at IS NOT NULL) FROM payments WHERE order_id=$O2 AND provider_payment_id IS NOT NULL")" "refunded true true"
check "history: why and who"                "$(events $O2) | $(q "SELECT note FROM order_events WHERE order_id=$O2 AND action='cancelled'")" "paid,cancelled,refund_needed,refunded | нет на складе"
check "  ...flash says money returned"      "$(flash_on $S/admin/orders/$O2 'деньги возвращены покупателю')" 1
check "bank page: money returned"           "$(curl -s "$U2" | grep -q 'Деньги возвращены' && echo yes)" yes
sleep 2
check "bank's refund webhook: no duplicate" "$(q "SELECT count(*) FROM order_events WHERE order_id=$O2 AND action='refunded'")" 1
check "  ...webhook delivered"              "$(grep -c "payment.refunded ${U2##*/} delivered" "$RUN/bankB.out")" 1
check "revenue counts only kept money"      "$(page $B $S/admin/ | grep -c 'stat-number">10 000 ₸')" 2

echo "== 4. bank unreachable during the refund → stays in the to-do list"
U3=$(buy); O3=$(last_order); paycard "$U3" "4242 4242 4242 4242" >/dev/null
wait_for "SELECT status FROM orders WHERE id=$O3" paid
stop_shop; start_shop http://127.0.0.1:8099 || echo "shop (broken bank URL) did not start"
post $B -d to=cancelled -d 'reason=test' $S/admin/orders/$O3/status >/dev/null
check "cancelled, money still owed"         "$(q "SELECT o.status||' '||p.status||' '||(p.refund_needed_at IS NOT NULL) FROM orders o JOIN payments p ON p.order_id=o.id AND p.status<>'failed' WHERE o.id=$O3")" "cancelled succeeded true"
check "  ...history: refund_failed"         "$(events $O3)" "paid,cancelled,refund_needed,refund_failed"
check "  ...flash explains"                 "$(flash_on $S/admin/orders/$O3 'банк не принял возврат')" 1
check "'refund' tab shows it"               "$(page $B "$S/admin/orders?status=refund" | grep -c "href=\"/admin/orders/$O3\"")" 1
check "dashboard warns"                     "$(page $B $S/admin/ | grep -c 'Нужно вернуть деньги по 1')" 1
stop_shop; start_shop $K || echo "shop did not restart"
P3=$(q "SELECT id FROM payments WHERE order_id=$O3 AND status='succeeded'")
check "other order's payment → 404"         "$(post $B -d payment_id=$P3 $S/admin/orders/$O1/refund)" 404
post $B -d payment_id=$P3 $S/admin/orders/$O3/refund >/dev/null
check "retry from the page works"           "$(q "SELECT status FROM payments WHERE id=$P3") $(events $O3)" "refunded paid,cancelled,refund_needed,refund_failed,refunded"
check "refund list is empty again"          "$(page $B $S/admin/ | grep -c 'Нужно вернуть деньги')" 0

echo "== 5. paid on a bank page AFTER the admin cancelled: refund, not revive"
U4=$(buy); O4=$(last_order)
post $B -d to=cancelled -d 'reason=duplicate' $S/admin/orders/$O4/status >/dev/null
check "pending order cancelled"             "$(q "SELECT status FROM orders WHERE id=$O4") $(q "SELECT failure_reason FROM payments WHERE order_id=$O4")" "cancelled cancelled"
paycard "$U4" "4242 4242 4242 4242" >/dev/null
wait_for "SELECT status FROM payments WHERE order_id=$O4" succeeded
check "order stays cancelled"               "$(q "SELECT status FROM orders WHERE id=$O4")" cancelled
check "payment flagged for refund"          "$(q "SELECT (refund_needed_at IS NOT NULL) FROM payments WHERE order_id=$O4")" t
check "  ...history explains"               "$(q "SELECT note FROM order_events WHERE order_id=$O4 AND action='refund_needed'" | grep -c 'после отмены администратором')" 1
P4=$(q "SELECT id FROM payments WHERE order_id=$O4")
post $B -d payment_id=$P4 $S/admin/orders/$O4/refund >/dev/null
check "refunded from the page"              "$(q "SELECT status FROM payments WHERE id=$P4")" refunded

echo "== 6. expired by timeout, then paid: the order comes back (phase 3 rule kept)"
U5=$(buy); O5=$(last_order)
echo "      waiting for the 20s reservation to run out…"
wait_for "SELECT status FROM orders WHERE id=$O5" cancelled 40
check "history: expired"                    "$(events $O5)" expired
PAY5=${U5##*/}; REF5=$(q "SELECT id FROM payments WHERE provider_payment_id='$PAY5'")
check "late success accepted"               "$(signed "$(ev payment.succeeded $PAY5 $REF5 1000000)" $(date +%s))" 200
check "order paid again"                    "$(q "SELECT status FROM orders WHERE id=$O5") $(events $O5)" "paid expired,paid"
U6=$(buy); O6=$(last_order); paycard "$U6" "4000 0000 0000 0002" >/dev/null
wait_for "SELECT status FROM payments WHERE order_id=$O6" failed
PAY6=${U6##*/}; REF6=$(q "SELECT id FROM payments WHERE provider_payment_id='$PAY6'")
check "refund event for money never taken"  "$(signed "$(ev payment.refunded $PAY6 $REF6 1000000)" $(date +%s)) $(q "SELECT status FROM payments WHERE id=$REF6")" "400 failed"

echo "== 7. users: roles and blocking"
AL=$(q "SELECT id FROM users WHERE email='alice@example.com'"); BO=$(q "SELECT id FROM users WHERE email='bob@example.com'"); ME=$(q "SELECT id FROM users WHERE email='boss@example.com'")
users=$(page $B $S/admin/users)
check "users page: alice's orders"          "$(echo "$users" | grep -c '>6</a>')" 1
check "  ...no password hashes on the page" "$(echo "$users" | grep -c '\$2a\$')" 0
post $B -d role=admin $S/admin/users/$BO/role >/dev/null
check "bob made admin"                      "$(q "SELECT role FROM users WHERE id=$BO")" admin
post $B -d action=block $S/admin/users/$BO/block >/dev/null
check "an admin can't be blocked"           "$(q "SELECT blocked_at IS NULL FROM users WHERE id=$BO") $(flash_on $S/admin/users 'снимите с пользователя роль')" "t 1"
post $B -d role=user $S/admin/users/$BO/role >/dev/null
check "bob demoted"                         "$(q "SELECT role FROM users WHERE id=$BO")" user
check "bob lost admin access at once"       "$(post -b "$RUN/bob.ck" $S/admin/)" 403
post $B -d role=user $S/admin/users/$ME/role >/dev/null
check "can't change own role"               "$(q "SELECT role FROM users WHERE id=$ME") $(flash_on $S/admin/users 'Свою роль менять нельзя')" "admin 1"
post $B -d role=superuser $S/admin/users/$BO/role >/dev/null
check "unknown role refused"                "$(flash_on $S/admin/users 'Неизвестная роль')" 1
post $B -d action=block $S/admin/users/$ME/block >/dev/null
check "can't block yourself"                "$(q "SELECT blocked_at IS NULL FROM users WHERE id=$ME")" t
post $B -d action=block $S/admin/users/$AL/block >/dev/null
check "alice blocked"                       "$(q "SELECT blocked_at IS NOT NULL FROM users WHERE id=$AL")" t
check "her open session ends at once"       "$(curl -s $A -o /dev/null -w '%{redirect_url}' $S/orders/)" "$S/auth/login?blocked=1"
r=$(cx 10.1.2.1 -w '\n%{http_code}' -d 'email=alice@example.com&password=supersecret1' $S/auth/login)
check "she can't log in"                    "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'заблокирован')" "403 1"
check "wrong password says nothing more"    "$(cx 10.1.2.2 -d 'email=alice@example.com&password=wrong-pass' $S/auth/login | grep -c 'заблокирован')" 0
post $B -d action=unblock $S/admin/users/$AL/block >/dev/null
check "unblocked: login works"              "$(cx 10.1.2.3 -o /dev/null -w '%{redirect_url}' -d 'email=alice@example.com&password=supersecret1' $S/auth/login)" "$S/clothes"

echo "== 8. migration 000007 down and up again"
stop_shop
# Roll back every migration from the newest down to 000007, newest first.
LATEST=$(ls "$REPO"/migrations/*.up.sql | wc -l)
downs_ok=ok
for f in $(cd "$REPO/migrations" && ls *.down.sql | sort -r); do
  [ "${f%%_*}" -lt 7 ] 2>/dev/null && continue
  MSYS_NO_PATHCONV=1 docker cp "$(cygpath -w "$REPO/migrations/$f")" pq-tb:/tmp/down.sql >/dev/null
  MSYS_NO_PATHCONV=1 docker exec pq-tb psql -U postgres -d planeta_qosshy -v ON_ERROR_STOP=1 -qf /tmp/down.sql >/dev/null 2>&1 || downs_ok="failed: $f"
done
check "down migrations run"                 "$downs_ok" ok
check "  ...refunded became succeeded"      "$(q "SELECT count(*) FROM payments WHERE status='refunded'")" 0
q "UPDATE schema_migrations SET version=6" >/dev/null
start_shop $K || echo "shop did not start after down"
check "up again: latest version"            "$(q "SELECT version||' '||dirty FROM schema_migrations")" "$LATEST false"

stop_shop; kill $BANK 2>/dev/null; taskkill //F //IM payments.exe >/dev/null 2>&1
docker rm -f pq-tb >/dev/null 2>&1
echo; echo "TOTAL: $pass passed, $fail failed"
