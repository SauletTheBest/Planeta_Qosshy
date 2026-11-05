#!/usr/bin/env bash
# Block C end-to-end: migration 000008 (old tokens hashed), cart, forgot/reset password,
# "log out everywhere", verification TTL + resend, product photo upload.
# Throw-away containers only: pq-tc (DB), pq-mail (Mailpit).
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
# Work folder for binaries, cookies, logs. NOT inside the repo: its path has a space and
# Cyrillic letters ("Рабочий стол"), which break unquoted arguments and curl.exe.
RUN="${RUN:-/tmp/planeta-e2e}"
RUNW=$(cygpath -m "$RUN")
S=http://127.0.0.1:8080; K=http://127.0.0.1:8081
KEY=$(openssl rand -hex 24); SECRET=$(openssl rand -hex 24); CS=$(openssl rand -base64 32)
UP="$RUN/uploads-test"
pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "PASS  $1 ($2)"; pass=$((pass+1)); else echo "FAIL  $1 (got '$2', want '$3')"; fail=$((fail+1)); fi; }
q() { docker exec pq-tc psql -U postgres -d planeta_qosshy -Atc "$1"; }
mp() { PYTHONIOENCODING=utf-8 python "$HERE/mp.py" "$@"; }
wait_for() { for i in $(seq 1 "${3:-20}"); do [ "$(q "$1")" = "$2" ] && return 0; sleep 1; done; return 1; }
cx() { local ip=$1; shift; curl -s -H "X-Forwarded-For: $ip" "$@"; }
jar() { echo "-b $RUN/$1.ck -c $RUN/$1.ck"; }       # a browser-like cookie jar
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
loc()  { curl -s -o /dev/null -w '%{redirect_url}' "$@"; }
login() { cx 10.3.$((ipn=ipn+1)).1 -o /dev/null -c "$RUN/$1.ck" -d "email=$1@example.com&password=${2:-supersecret1}" $S/auth/login; }
flash() { curl -s $(jar $1) "$2" | grep -c "$3"; }
paycard() { curl -s -o /dev/null -w '%{redirect_url}' --data-urlencode "card_number=$2" -d 'expiry=12/30&cvv=123&holder=IVAN' "$1"; }
stock() { q "SELECT stock FROM clothes_variants WHERE clothes_id=$1 AND size='$2'"; }

start_shop() {  # $1 = binary
  if [ "$1" = "$RUN/app7.exe" ]; then cd "$RUN/v7-src"; else cd "$REPO"; fi
  env "DATABASE_URL=host=127.0.0.1 port=55443 user=postgres password=test dbname=planeta_qosshy sslmode=disable" \
    COOKIE_SECRET=$CS PAYMENT_API_URL=$K PAYMENT_API_KEY=$KEY PAYMENT_WEBHOOK_SECRET=$SECRET \
    PUBLIC_URL=$S VERIFICATION_ADDRESS=$S TRUSTED_PROXIES=127.0.0.1 UPLOAD_DIR="$RUNW/uploads-test" \
    SMTP_HOST=127.0.0.1 SMTP_PORT=1025 SMTP_USER=shop@test.local SMTP_PASS=x \
    ORDER_RESERVATION_TTL=20s ORDER_EXPIRY_GRACE=2s ORDER_EXPIRY_INTERVAL=2s \
    "$1" >> "$RUN/shopC.out" 2>&1 & APP=$!
  cd "$RUN"
  for i in $(seq 1 40); do curl -s -o /dev/null $S/ && return 0; sleep 0.5; done; return 1
}
stop_shop() { kill $APP 2>/dev/null; wait $APP 2>/dev/null; taskkill //F //IM app.exe >/dev/null 2>&1; taskkill //F //IM app7.exe >/dev/null 2>&1; sleep 0.5; }

: > "$RUN/shopC.out"; rm -rf "$UP"; : # test images are made by build.sh
printf 'not an image at all' > "$RUN/fake.png"
docker run -d --rm --name pq-mail -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 axllent/mailpit:latest >/dev/null
docker run -d --rm --name pq-tc -e POSTGRES_PASSWORD=test -e POSTGRES_DB=planeta_qosshy -p 127.0.0.1:55443:5432 --memory 256m postgres:16-alpine >/dev/null
for i in $(seq 1 30); do docker exec pq-tc pg_isready -U postgres -d planeta_qosshy >/dev/null 2>&1 && break; sleep 1; done; sleep 2
mkdir -p "$RUN/bank"; cd "$RUN/bank"
PORT=8081 PUBLIC_URL=$K API_KEY=$KEY WEBHOOK_SECRET=$SECRET MERCHANT_WEBHOOK_URL=$S/payments/webhook MERCHANT_SITE_URL=$S \
  "$RUN/payments.exe" > "$RUN/bankC.out" 2>&1 & BANK=$!
cd "$RUN"

echo "== 0. migration 000008 hashes confirmation tokens that were already emailed"
start_shop "$RUN/app7.exe" || echo "v7 shop did not start"
cx 10.3.0.1 -o /dev/null -d 'username=olduser&email=olduser@example.com&password=supersecret1' $S/auth/register
OLDTOKEN=$(q "SELECT verification_token FROM users WHERE email='olduser@example.com'")
check "v7 stored the raw token"             "${#OLDTOKEN}" 44
stop_shop
start_shop "$RUN/app.exe" || echo "shop did not start"
check "schema at version 8"                 "$(q "SELECT version||' '||dirty FROM schema_migrations")" "8 false"
check "token in DB is now a hash"           "$(q "SELECT length(verification_token)||' '||(verification_token <> '$OLDTOKEN')||' '||(verification_sent_at IS NOT NULL) FROM users WHERE email='olduser@example.com'")" "64 true true"
check "the link from the old email works"   "$(loc "$S/auth/verify?token=$OLDTOKEN")" "$S/auth/login?verified=1"
check "  ...user verified, token cleared"   "$(q "SELECT verified||' '||coalesce(verification_token,'') FROM users WHERE email='olduser@example.com'")" "true "

echo "== setup"
for u in boss alice bob carol; do cx 10.3.0.$((rn=rn+10)) -o /dev/null -d "username=$u&email=$u@example.com&password=supersecret1" $S/auth/register; done
q "UPDATE users SET role='admin' WHERE email='boss@example.com'" >/dev/null
for u in boss alice bob; do login $u; done
code $(jar boss) --data-urlencode "size=S:2, M:1" -d 'price=5000&title=Hoodie&brand=Q&category=Худи&color=Red&image_url=&description=x' $S/admin/clothes >/dev/null
code $(jar boss) --data-urlencode "size=One size:5" -d 'price=1000&title=Socks&brand=Q&category=Носки&color=Red&image_url=&description=x' $S/admin/clothes >/dev/null
P=$(q "SELECT id FROM clothes WHERE title='Hoodie'"); Q=$(q "SELECT id FROM clothes WHERE title='Socks'")
VS=$(q "SELECT id FROM clothes_variants WHERE clothes_id=$P AND size='S'")

echo "== 1. cart"
A=$(jar alice)
check "product page has 'В корзину'"        "$(curl -s $A $S/clothes/$P | grep -c 'action="/cart/add"')" 1
code $A -d "clothes_id=$P&size=S&quantity=1" $S/cart/add >/dev/null
code $A -d "clothes_id=$P&size=S&quantity=2" $S/cart/add >/dev/null
check "same size twice = one line, qty 3"   "$(q "SELECT count(*)||' '||sum(quantity) FROM cart_items")" "1 3"
check "  ...page warns: only 2 left"        "$(curl -s $A $S/cart/ | grep -c 'осталось только 2 шт.')" 1
code $A -d "clothes_id=$P&size=S&quantity=50" $S/cart/add >/dev/null
check "quantity capped at 10"               "$(q "SELECT quantity FROM cart_items WHERE variant_id=$VS")" 10
code $A -d "clothes_id=$P&size=XL&quantity=1" $S/cart/add >/dev/null
check "unknown size: message"               "$(flash alice $S/cart/ 'Выберите размер товара')" 1
code $A -d "variant_id=$VS&quantity=2" $S/cart/update >/dev/null
code $A -d "clothes_id=$P&size=M&quantity=1" $S/cart/add >/dev/null
code $A -d "clothes_id=$Q&quantity=3" $S/cart/add >/dev/null       # one size: no size needed
check "3 lines, total 18 000 ₸"             "$(q "SELECT count(*) FROM cart_items") $(curl -s $A $S/cart/ | grep -c 'Итого.*18 000 ₸\|на 18 000 ₸')" "3 2"
check "nothing reserved by the cart"        "$(stock $P S) $(stock $P M) $(stock $Q 'One size')" "2 1 5"
code $(jar bob) -d "variant_id=$VS&quantity=0" $S/cart/update >/dev/null
check "bob can't touch alice's cart"        "$(q "SELECT quantity FROM cart_items WHERE variant_id=$VS")" 2
loc $(jar bob) -d 'size=S' $S/payment/$P >/dev/null                 # bob buys one S meanwhile
check "bob took one S"                      "$(stock $P S)" 1
code $A -X POST $S/cart/checkout >/dev/null
check "checkout refused: names the item"    "$(flash alice $S/cart/ 'Hoodie» (S): осталось только 1 шт.')" 1
check "  ...all or nothing: nothing taken"  "$(stock $P S) $(stock $P M) $(stock $Q 'One size') $(q "SELECT count(*) FROM cart_items")" "1 1 5 3"
check "  ...no order for alice"             "$(q "SELECT count(*) FROM orders o JOIN users u ON u.id=o.user_id WHERE u.email='alice@example.com'")" 0
code $A -d "variant_id=$VS&quantity=1" $S/cart/update >/dev/null
U=$(loc $A -X POST $S/cart/checkout)
check "checkout → bank page"                "${U%/pay_*}" "$K/pay"
O=$(q "SELECT max(o.id) FROM orders o JOIN users u ON u.id=o.user_id WHERE u.email='alice@example.com'")
check "one order, 3 lines, 13 000 ₸"        "$(q "SELECT count(*)||' '||sum(quantity) FROM order_items WHERE order_id=$O") $(q "SELECT total_tiyn FROM orders WHERE id=$O")" "3 5 1300000"
check "stock reserved, cart emptied"        "$(stock $P S) $(stock $P M) $(stock $Q 'One size') $(q "SELECT count(*) FROM cart_items")" "0 0 2 0"
paycard "$U" "4242 4242 4242 4242" >/dev/null
wait_for "SELECT status FROM orders WHERE id=$O" paid
check "paid"                                "$(q "SELECT status FROM orders WHERE id=$O")" paid
check "admin sees all 3 lines"              "$(curl -s $(jar boss) $S/admin/orders/$O | grep -c 'Hoodie\|Socks')" 3
check "profile shows the purchases"         "$([ "$(curl -s $A $S/profile/ | grep -c 'Hoodie\|Socks')" -ge 3 ] && echo yes)" yes
code $A -d "clothes_id=$Q&quantity=2" $S/cart/add >/dev/null
loc $A -X POST $S/cart/checkout >/dev/null
O2=$(q "SELECT max(id) FROM orders")
check "unpaid cart order reserves"          "$(stock $Q 'One size')" 0
echo "      waiting for the 20s reservation to run out…"
wait_for "SELECT status FROM orders WHERE id=$O2" cancelled 40
check "expired: every line released"        "$(stock $Q 'One size')" 2
code $A -d "clothes_id=$Q&quantity=1" $S/cart/add >/dev/null
code $(jar boss) --data-urlencode "size=L:3" -d "price=1000&title=Socks&brand=Q&category=Носки&color=Red&image_url=&description=x" $S/admin/clothes/$Q >/dev/null
check "admin removed the size → cart line"  "$(q "SELECT count(*) FROM cart_items")" 0

echo "== 2. forgot / reset password"
r=$(cx 10.3.9.1 -d 'email=nobody@example.com' $S/auth/forgot)
check "unknown email: same polite answer"   "$(echo "$r" | grep -c 'Если аккаунт')" 1
cx 10.3.9.2 -o /dev/null -d 'email=ALICE@example.com' $S/auth/forgot; sleep 1
LINK1=$(mp alice@example.com link)
check "letter with a link arrived"          "${LINK1%%\?*}" "$S/auth/reset"
check "DB has only a hash of the token"     "$(q "SELECT count(*)||' '||(min(token_hash) <> '${LINK1##*token=}')||' '||length(min(token_hash)) FROM password_resets")" "1 true 64"
check "nobody@ got no letter"               "$(mp nobody@example.com subject)" NONE
cx 10.3.9.3 -o /dev/null -d 'email=alice@example.com' $S/auth/forgot; sleep 1
LINK2=$(mp alice@example.com link)
check "asking again: only the new link"     "$(q "SELECT count(*) FROM password_resets WHERE used_at IS NULL") $(code "$LINK1") $(code "$LINK2")" "1 400 200"
T2=${LINK2##*token=}
check "passwords must match"                "$(code -d "token=$T2&password=newsecret11&password2=other" $S/auth/reset)" 400
check "too short"                           "$(code -d "token=$T2&password=short&password2=short" $S/auth/reset)" 400
check "alice's session works before"        "$(code $A $S/orders/)" 200
check "reset → login page"                  "$(loc -d "token=$T2&password=newsecret11&password2=newsecret11" $S/auth/reset)" "$S/auth/login?reset=1"
check "her old session is logged out"       "$(loc -b "$RUN/alice.ck" $S/orders/)" "$S/auth/login"
check "old password no longer works"        "$(cx 10.3.9.4 -o /dev/null -w '%{http_code}' -d 'email=alice@example.com&password=supersecret1' $S/auth/login)" 401
check "new password works"                  "$(cx 10.3.9.5 -o /dev/null -w '%{redirect_url}' -c "$RUN/alice.ck" -d 'email=alice@example.com&password=newsecret11' $S/auth/login)" "$S/clothes"
check "link works only once"                "$(code "$LINK2")" 400
cx 10.3.9.6 -o /dev/null -d 'email=bob@example.com' $S/auth/forgot; sleep 1
q "UPDATE password_resets SET expires_at = now() - interval '1 minute' WHERE used_at IS NULL" >/dev/null
check "expired link refused"                "$(code "$(mp bob@example.com link)")" 400
q "UPDATE users SET blocked_at = now() WHERE email='carol@example.com'" >/dev/null
cx 10.3.9.7 -o /dev/null -d 'email=carol@example.com' $S/auth/forgot; sleep 1
check "blocked user gets no reset letter"   "$(mp carol@example.com subject)" "Verify Your Email — Planeta Qosshy"
q "UPDATE users SET blocked_at = NULL WHERE email='carol@example.com'" >/dev/null

echo "== 3. password change in the profile: this browser stays, others are out"
cx 10.3.10.1 -o /dev/null -c "$RUN/bob2.ck" -d 'email=bob@example.com&password=supersecret1' $S/auth/login
sleep 1.1   # a new second: sessions store their start time in whole seconds
code $(jar bob) -d 'username=bob&email=bob@example.com&password=bobsecret22&current_password=supersecret1' $S/profile/edit >/dev/null
check "the browser that changed it: in"     "$(code $(jar bob) $S/orders/)" 200
check "bob's other browser: logged out"     "$(loc -b "$RUN/bob2.ck" $S/orders/)" "$S/auth/login"

echo "== 4. email confirmation: hashed, 24 h, resend"
CL=$(mp carol@example.com link)
check "DB keeps a hash, not the link token" "$(q "SELECT verification_token <> '${CL##*token=}' AND length(verification_token)=64 FROM users WHERE email='carol@example.com'")" t
q "UPDATE users SET verification_sent_at = now() - interval '25 hours' WHERE email='carol@example.com'" >/dev/null
r=$(curl -s -w '\n%{http_code}' "$CL")
check "25 h old link: refused, says why"    "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'устарела')" "400 1"
login carol
code $(jar carol) -X POST $S/profile/verify >/dev/null; sleep 0.5
check "resend: message in the profile"      "$(flash carol $S/profile/ 'Письмо отправлено на carol@example.com')" 1
CL2=$(mp carol@example.com link)
check "new link works, old one doesn't"     "$(code "$CL") $(loc "$CL2")" "400 $S/auth/login?verified=1"
code $(jar carol) -X POST $S/profile/verify >/dev/null
check "already verified: says so"           "$(flash carol $S/profile/ 'уже подтверждён')" 1

echo "== 5. product photo upload"
B=$(jar boss)
r=$(curl -s $B -w '\n%{http_code}' -F 'title=Photo tee' -F 'brand=Q' -F 'category=Футболки' -F 'color=White' -F 'price=3000' -F 'size=M:1' -F 'description=x' -F "image=@$RUNW/fake.png" $S/admin/clothes)
check "renamed text file refused"           "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'можно только JPG или PNG')" "400 1"
r=$(curl -s $B -w '\n%{http_code}' -F 'title=Photo tee' -F 'brand=Q' -F 'category=Футболки' -F 'color=White' -F 'price=3000' -F 'size=M:1' -F 'description=x' -F "image=@$RUNW/bomb.png" $S/admin/clothes)
check "100000×100000 'bomb' refused"        "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'слишком большое разрешение')" "400 1"
check "  ...and nothing was saved"          "$(ls "$UP/clothes" 2>/dev/null | wc -l) $(q "SELECT count(*) FROM clothes WHERE title='Photo tee'")" "0 0"
code $B -F 'title=Photo tee' -F 'brand=Q' -F 'category=Футболки' -F 'color=White' -F 'price=3000' -F 'size=M:1' -F 'description=x' -F "image=@$RUNW/small.png" $S/admin/clothes >/dev/null
IMG=$(q "SELECT image_url FROM clothes WHERE title='Photo tee'"); T=$(q "SELECT id FROM clothes WHERE title='Photo tee'")
check "stored under a random name"          "$(echo "$IMG" | grep -cE '^/uploads/clothes/[0-9a-f]{32}\.png$')" 1
hdr=$(curl -s -D - -o /dev/null "$S$IMG" | tr -d '\r')
check "served as PNG with nosniff"          "$(echo "$hdr" | grep -ci '^content-type: image/png') $(echo "$hdr" | grep -ci '^x-content-type-options: nosniff')" "1 1"
code $B -F 'title=Photo tee' -F 'brand=Q' -F 'category=Футболки' -F 'color=White' -F 'price=3000' -F 'size=M:1' -F 'description=x' -F "image=@$RUNW/small2.png" $S/admin/clothes/$T >/dev/null
IMG2=$(q "SELECT image_url FROM clothes WHERE id=$T")
check "new photo replaces the old file"     "$([ "$IMG2" != "$IMG" ] && echo changed) $(ls "$UP/clothes" | wc -l) $(code "$S$IMG") $(code "$S$IMG2")" "changed 1 404 200"
code $B -d "price=3000&title=Photo tee&brand=Q&category=Футболки&color=White&image_url=$IMG2&description=y&size=M:1" $S/admin/clothes/$T >/dev/null
check "plain form w/o file still saves"     "$(q "SELECT description||' '||(image_url='$IMG2') FROM clothes WHERE id=$T")" "y true"
code $B -X POST $S/admin/clothes/$T/delete >/dev/null
check "deleting the product removes photo"  "$(ls "$UP/clothes" | wc -l)" 0
check "no path tricks in /uploads"          "$(code --path-as-is "$S/uploads/../main.go")" 404

stop_shop; kill $BANK 2>/dev/null; taskkill //F //IM payments.exe >/dev/null 2>&1
docker rm -f pq-tc pq-mail >/dev/null 2>&1
echo; echo "TOTAL: $pass passed, $fail failed"
