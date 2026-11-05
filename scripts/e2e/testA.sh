#!/usr/bin/env bash
# Block A end-to-end: migration 000006, email case, per-IP rate limit, pagination,
# profile, helpdesk attachments, logs. Throw-away containers only: pq-ta, pq-ta2, pq-mail.
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
# Work folder for binaries, cookies, logs. NOT inside the repo: its path has a space and
# Cyrillic letters ("Рабочий стол"), which break unquoted arguments and curl.exe.
RUN="${RUN:-/tmp/planeta-e2e}"
S=http://127.0.0.1:8080
pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "PASS  $1 ($2)"; pass=$((pass+1)); else echo "FAIL  $1 (got '$2', want '$3')"; fail=$((fail+1)); fi; }
q()  { docker exec "${DBC:-pq-ta}" psql -U postgres -d planeta_qosshy -Atc "$1"; }
mp() { PYTHONIOENCODING=utf-8 python "$HERE/mp.py" "$@"; }
# curl from a pretend client IP (works only while TRUSTED_PROXIES=127.0.0.1)
cx() { local ip=$1; shift; curl -s -H "X-Forwarded-For: $ip" "$@"; }
code() { local ip=$1; shift; cx "$ip" -o /dev/null -w '%{http_code}' "$@"; }

start() {  # $1 = binary, $2 = DB port, $3 = log, rest = extra env
  local bin=$1 port=$2 log=$3; shift 3
  # the old code writes server.log into its working dir, so it runs from its own copy
  if [ "$bin" = "$RUN/old.exe" ]; then cd "$RUN/old-src"; else cd "$REPO"; fi
  env "DATABASE_URL=host=127.0.0.1 port=$port user=postgres password=test dbname=planeta_qosshy sslmode=disable" \
    COOKIE_SECRET=$CS PAYMENT_API_URL=http://127.0.0.1:8081 PAYMENT_API_KEY=$(openssl rand -hex 24) \
    PAYMENT_WEBHOOK_SECRET=$(openssl rand -hex 24) PUBLIC_URL=$S VERIFICATION_ADDRESS=$S \
    SMTP_HOST=127.0.0.1 SMTP_PORT=1025 SMTP_USER=shop@test.local SMTP_PASS=x SUPPORT_EMAIL=support@test.local \
    "$@" "$bin" > "$log" 2>&1 &
  APP=$!
  cd "$RUN"
  for i in $(seq 1 40); do curl -s -o /dev/null "$S/" && return 0; kill -0 $APP 2>/dev/null || return 1; sleep 0.5; done
  return 1
}
stop() { kill $APP 2>/dev/null; wait $APP 2>/dev/null; taskkill //F //IM app.exe >/dev/null 2>&1; taskkill //F //IM old.exe >/dev/null 2>&1; sleep 0.5; }
newdb() {  # $1 name, $2 port
  docker run -d --rm --name "$1" -e POSTGRES_PASSWORD=test -e POSTGRES_DB=planeta_qosshy -p "127.0.0.1:$2:5432" --memory 256m postgres:16-alpine >/dev/null
  for i in $(seq 1 30); do docker exec "$1" pg_isready -U postgres -d planeta_qosshy >/dev/null 2>&1 && break; sleep 1; done; sleep 2
}
CS=$(openssl rand -base64 32)

docker run -d --rm --name pq-mail -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 axllent/mailpit:latest >/dev/null
newdb pq-ta 55440
newdb pq-ta2 55441

echo "== 0. migration 000006 on a database filled by the OLD code"
start "$RUN/old.exe" 55440 "$RUN/oldA.out" || echo "old app did not start"
curl -s -o /dev/null -d 'username=mixed&email=Mixed@Case.KZ&password=supersecret1' $S/auth/register; sleep 1.2
stop
DBC=pq-ta2 start "$RUN/old.exe" 55441 "$RUN/oldA2.out" || echo "old app 2 did not start"
curl -s -o /dev/null -d 'username=dup1&email=Dup@x.kz&password=supersecret1' $S/auth/register; sleep 1.2
curl -s -o /dev/null -d 'username=dup2&email=dup@x.kz&password=supersecret1' $S/auth/register; sleep 1.2
stop
check "old code stored the email as typed"  "$(q "SELECT email FROM users WHERE username='mixed'")" "Mixed@Case.KZ"
check "old code allowed two Dup emails"     "$(DBC=pq-ta2 q "SELECT count(*) FROM users WHERE lower(email)='dup@x.kz'")" 2

start "$RUN/app.exe" 55441 "$RUN/a2.out"; started=$?
check "clash: new app refuses to start"     "$started" 1
check "clash: log names the emails"         "$(grep -q 'differ only by letter case: dup@x.kz' "$RUN/a2.out" && echo yes)" yes
check "clash: version 6 marked dirty"     "$(DBC=pq-ta2 q "SELECT version||' '||dirty FROM schema_migrations")" "6 true"
check "clash: data untouched"               "$(DBC=pq-ta2 q "SELECT string_agg(email, ',' ORDER BY id) FROM users")" "Dup@x.kz,dup@x.kz"
stop

start "$RUN/app.exe" 55440 "$RUN/a.out" TRUSTED_PROXIES=127.0.0.1 || echo "new app did not start"
check "schema at version 6 or later"        "$(q "SELECT version >= 6 AND NOT dirty FROM schema_migrations")" t
check "existing email lower-cased"          "$(q "SELECT email FROM users WHERE username='mixed'")" "mixed@case.kz"
check "DB refuses a mixed-case email"       "$(q "INSERT INTO users (username,email,password) VALUES ('zz','Upper@x.kz','h')" 2>&1 | grep -c users_email_lowercase)" 1
check "old user logs in with ANY case"      "$(cx 10.0.0.9 -o /dev/null -w '%{redirect_url}' -d 'email=MIXED@case.kz&password=supersecret1' $S/auth/login)" "$S/clothes"

echo "== 1. registration"
cx 10.0.1.1 -o /dev/null -d 'username=alice&email= Alice@Example.COM &password=supersecret1' $S/auth/register
cx 10.0.1.2 -o /dev/null -d 'username=bob&email=bob@example.com&password=supersecret2' $S/auth/register
check "email saved lower-case, trimmed"     "$(q "SELECT email FROM users WHERE username='alice'")" "alice@example.com"
r=$(cx 10.0.1.3 -w '\n%{http_code}' -d 'username=alice9&email=ALICE@example.com&password=supersecret1' $S/auth/register)
check "same email other case: 409"          "$(echo "$r" | tail -1)" 409
check "  ...with a clear message"           "$(echo "$r" | grep -c 'Этот email уже зарегистрирован')" 1
r=$(cx 10.0.1.4 -w '\n%{http_code}' -d 'username=bob&email=bob2@example.com&password=supersecret1' $S/auth/register)
check "taken username: 409 + message"       "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'имя пользователя уже занято')" "409 1"
r=$(cx 10.0.1.5 -d 'username=ab&email=ab@example.com&password=supersecret1' $S/auth/register)
check "short name: form kept + reason"      "$(echo "$r" | grep -c 'value="ab@example.com"\|от 3 до 20')" 2
printf 'username=Саулет&email=saulet@example.com&password=supersecret1' > "$RUN/cyr.txt"
check "Cyrillic name of 6 letters is ok"    "$(code 10.0.1.6 --data-binary @"$(cygpath -m "$RUN/cyr.txt")" -H 'Content-Type: application/x-www-form-urlencoded' $S/auth/register)" 200
check "  ...stored as real Cyrillic"        "$(q "SELECT username FROM users WHERE email='saulet@example.com'")" "Саулет"
check "verification letter sent"            "$(mp alice@example.com subject)" "Verify Your Email — Planeta Qosshy"
cx 10.0.2.1 -o /dev/null -c "$RUN/alice.ck" -d 'email=ALICE@EXAMPLE.COM&password=supersecret1' $S/auth/login
check "login with other case works"         "$(curl -s -o /dev/null -w '%{http_code}' -b "$RUN/alice.ck" $S/profile/)" 200

echo "== 2. rate limit per IP"
codes=""; for i in 1 2 3 4 5 6; do codes="$codes $(code 10.0.3.1 -d 'email=alice@example.com&password=wrong-pass' $S/auth/login)"; done
check "5 wrong logins, the 6th is 429"      "$codes" " 401 401 401 401 401 429"
check "429 says when to retry"              "$(cx 10.0.3.1 -o /dev/null -D - -d 'email=a@b.kz&password=x' $S/auth/login | tr -d '\r' | grep -i '^retry-after')" "Retry-After: 12"
check "another IP is not blocked"           "$(code 10.0.3.2 -d 'email=alice@example.com&password=wrong-pass' $S/auth/login)" 401
ok=0; for i in $(seq 1 40); do [ "$(code 10.0.3.1 $S/clothes/)" = 200 ] && ok=$((ok+1)); done
check "40 quick catalog pages all load"     "$ok" 40

echo "== 3. catalog pages"
q "WITH c AS (INSERT INTO clothes (title, brand, category, color, price_tiyn, image_url, description, created_at)
              SELECT 'Item '||g, 'B', 'Cat', 'Red', 100000+g, '', 'd', now() FROM generate_series(1,25) g RETURNING id)
   INSERT INTO clothes_variants (clothes_id, size, stock) SELECT id, 'M', 1 FROM c" >/dev/null
items() { grep -o 'href="/clothes/[0-9]*"' | sort -u | wc -l; }
p1=$(curl -s "$S/clothes/?limit=12")
check "25 items: page 1 of 3"               "$(echo "$p1" | grep -c 'Страница 1 из 3') $(echo "$p1" | items)" "1 12"
p3=$(curl -s "$S/clothes/?page=3&limit=12")
check "page 3 of 3: 1 item, no Next"        "$(echo "$p3" | grep -c 'Страница 3 из 3') $(echo "$p3" | items) $(echo "$p3" | grep -c 'Вперёд')" "1 1 0"
check "?limit=0 no longer crashes"          "$(curl -s -o /dev/null -w '%{http_code}' "$S/clothes/?limit=0")" 200
check "?limit=1000000 gives 12 items"       "$(curl -s "$S/clothes/?limit=1000000" | items)" 12
check "?page=-5 is page 1"                  "$(curl -s "$S/clothes/?page=-5" | grep -c 'Страница 1 из 3')" 1
r=$(curl -s -w '\n%{http_code}' $S/clothes/999999)
check "missing product: HTML 404"           "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'Товар не найден')" "404 1"

echo "== 4. profile"
# -c too: a password change gives this browser a NEW session cookie (old sessions die)
A="-b $RUN/alice.ck -c $RUN/alice.ck"
check "edit form posts to /profile/edit"    "$(curl -s $A $S/profile/edit | grep -c 'action="/profile/edit"')" 1
check "name change needs no password"       "$(curl -s $A -o /dev/null -w '%{http_code}' -d 'username=alice2&email=alice@example.com' $S/profile/edit)" 303
check "  ...name changed"                   "$(q "SELECT username FROM users WHERE email='alice@example.com'")" alice2
r=$(curl -s $A -w '\n%{http_code}' -d 'username=alice2&email=new@example.com' $S/profile/edit)
check "email change w/o password: 403"      "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'введите текущий пароль')" "403 1"
check "wrong current password: 403"         "$(curl -s $A -o /dev/null -w '%{http_code}' -d 'username=alice2&email=new@example.com&current_password=nope-nope' $S/profile/edit)" 403
check "  ...email not changed"              "$(q "SELECT count(*) FROM users WHERE email='new@example.com'")" 0
check "email change with password: 303"     "$(curl -s $A -o /dev/null -w '%{http_code}' -d 'username=alice2&email=New@Example.com&current_password=supersecret1' $S/profile/edit)" 303
check "  ...new email unverified, token"    "$(q "SELECT email||' '||verified||' '||(length(verification_token)>20) FROM users WHERE username='alice2'")" "new@example.com false true"
check "  ...letter sent to the new email"   "$(mp new@example.com subject)" "Verify Your Email — Planeta Qosshy"
r=$(curl -s $A -w '\n%{http_code}' -d 'username=alice2&email=bob@example.com&current_password=supersecret1' $S/profile/edit)
check "someone else's email: 409"           "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'уже зарегистрирован')" "409 1"
check "password change: 303"                "$(curl -s $A -o /dev/null -w '%{http_code}' -d 'username=alice2&email=new@example.com&password=supersecret9&current_password=supersecret1' $S/profile/edit)" 303
check "  ...new password works"             "$(cx 10.0.4.1 -o /dev/null -w '%{redirect_url}' -d 'email=new@example.com&password=supersecret9' $S/auth/login)" "$S/clothes"
check "old URL /profile/edit/:id is gone"   "$(curl -s $A -o /dev/null -w '%{http_code}' -d 'username=x' $S/profile/edit/1)" 404

echo "== 5. helpdesk"
printf 'just text' > "$RUN/fake.jpg"
printf '\x89PNG\r\n\x1a\n0000000000000000' > "$RUN/real.png"
printf '%%PDF-1.4\n%%EOF\n' > "$RUN/real.pdf"
head -c 6000000 /dev/zero > "$RUN/big.png"; printf '\x89PNG\r\n\x1a\n' | dd of="$RUN/big.png" conv=notrunc 2>/dev/null
head -c 17000000 /dev/zero > "$RUN/huge.bin"
H() { local ip=$1; shift; cx "$ip" $A -w '\n%{http_code}' "$@" $S/helpdesk/send; }
check "form shows the account email"        "$(curl -s $A $S/helpdesk/ | grep -c 'new@example.com')" 1
r=$(H 10.0.5.1 -F 'subject=' -F 'message=hi')
check "empty subject: 400"                  "$(echo "$r" | tail -1)" 400
r=$(H 10.0.5.2 -F 'subject=Hi' -F 'message=hi' -F "attachments=@$RUN/fake.jpg")
check "renamed text file: 400"              "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'можно только JPG, PNG или PDF')" "400 1"
r=$(H 10.0.5.3 -F 'subject=Hi' -F 'message=hi' -F "attachments=@$RUN/real.png" -F "attachments=@$RUN/real.png" -F "attachments=@$RUN/real.png" -F "attachments=@$RUN/real.png")
check "4 files: 400"                        "$(echo "$r" | tail -1)" 400
r=$(H 10.0.5.4 -F 'subject=Hi' -F 'message=hi' -F "attachments=@$RUN/big.png")
check "6 MB file: 400 + reason"             "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'больше 5 МБ')" "400 1"
r=$(H 10.0.5.5 -F 'subject=Hi' -F 'message=hi' -F "attachments=@$RUN/huge.bin")
check "17 MB body: 413"                     "$(echo "$r" | tail -1)" 413
printf 'Где мой заказ' > "$RUN/subj.txt"; printf 'Здравствуйте!' > "$RUN/msg.txt"
r=$(H 10.0.5.6 -F "subject=<$(cygpath -m "$RUN/subj.txt")" -F "message=<$(cygpath -m "$RUN/msg.txt")" -F "attachments=@$(cygpath -m "$RUN/real.png");filename=../../evil name.png" -F "attachments=@$RUN/real.pdf")
check "valid message: 200 + success"        "$(echo "$r" | tail -1) $(echo "$r" | grep -c 'Сообщение отправлено')" "200 1"
check "  ...arrived at support"             "$(mp support@test.local subject)" "[Поддержка] Где мой заказ"
check "  ...Reply-To is the customer"       "$(mp support@test.local replyto)" "new@example.com"
check "  ...both files, our own names"      "$(mp support@test.local files)" "attachment-1.png,attachment-2.pdf"
check "nothing written to uploads/"         "$([ -e "$REPO/uploads" ] && echo exists || echo none)" none
codes=""; for i in 1 2 3 4; do codes="$codes $(code 10.0.5.9 $A -F 'subject=' -F 'message=x' $S/helpdesk/send)"; done
check "helpdesk: 3 per IP, then 429"        "$codes" " 400 400 400 429"

echo "== 6. logs"
id=$(curl -s -o /dev/null -D - "$S/auth/verify?token=SECRET-TOKEN-123" | tr -d '\r' | sed -n 's/^X-Request-Id: //Ip')
check "response carries X-Request-ID"       "${#id}" 16
sleep 0.3
check "exactly one log line per request"    "$(grep -c "$id" "$RUN/a.out")" 1
check "  ...it is JSON with the path"       "$(grep "$id" "$RUN/a.out" | python -c 'import json,sys; d=json.loads(sys.stdin.read()); print(d["path"], d["status"], d["msg"])')" "/auth/verify 400 request"
check "query secrets not logged"            "$(grep -c 'SECRET-TOKEN-123' "$RUN/a.out")" 0
check "no server.log file in the repo"      "$([ -e "$REPO/server.log" ] && echo exists || echo none)" none

echo "== 7. X-Forwarded-For can't be faked without TRUSTED_PROXIES"
stop
start "$RUN/app.exe" 55440 "$RUN/a3.out" || echo "app (no proxies) did not start"
codes=""; for i in 1 2 3 4 5 6; do codes="$codes $(code 10.9.9.$i -d 'email=a@b.kz&password=wrong-pass' $S/auth/login)"; done
check "6 'different' IPs = one client"      "$codes" " 401 401 401 401 401 429"

stop
docker rm -f pq-ta pq-ta2 pq-mail >/dev/null 2>&1
echo; echo "TOTAL: $pass passed, $fail failed"
