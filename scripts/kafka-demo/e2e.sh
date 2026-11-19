#!/usr/bin/env bash
# Kafka end-to-end check on a THROW-AWAY copy of the docker-compose stand.
#
#   bash scripts/kafka-demo/e2e.sh          # build, run every check, delete the copy
#   KEEP=1 bash scripts/kafka-demo/e2e.sh   # ...but leave the copy running to look around
#   bash scripts/kafka-demo/e2e.sh down     # delete a copy left by KEEP=1
#
# The copy is the compose project "pq-kafka-e2e": its own containers (pq-e2e-*), its own
# volumes and other ports, so your stand (planeta_qosshy_*, its database) is not touched:
#   shop http://localhost:18090   bank :18081   Mailpit :18025   kafka-ui :18085
#
# What it shows:
#   1. an order is paid → event: shop → outbox_events → Kafka → notifier → letter
#   2. notifier stopped → Kafka keeps the event (lag 1); started again → it catches up
#   3. Kafka stopped → the shop still sells, events wait in outbox_events; Kafka back → sent
#   4. the notifier reads the WHOLE topic again (offset reset) → no duplicate letters
#   5. admin cancels a paid order → «отменён» + «деньги возвращены»; one order = one partition
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
W="${W:-/tmp/pq-kafka-e2e}"    # generated compose file, env, cookies — outside the repo
mkdir -p "$W"
P=pq-kafka-e2e
S=http://127.0.0.1:18090; UI=http://127.0.0.1:18085
export MAILPIT_API=http://127.0.0.1:18025/api/v1
pass=0; fail=0

check() { if [ "$2" = "$3" ]; then echo "PASS  $1 ($2)"; pass=$((pass+1)); else echo "FAIL  $1 (got '$2', want '$3')"; fail=$((fail+1)); fi; }
# Git Bash rewrites "/opt/..." arguments into Windows paths; docker must get them as they are.
dk() { MSYS_NO_PATHCONV=1 docker "$@"; }
dc() { dk compose -p $P -f "$(cygpath -m "$W/compose.yml")" --project-directory "$(cygpath -m "$REPO")" \
          --env-file "$(cygpath -m "$W/e2e.env")" --profile dev "$@"; }
q()  { dk exec pq-e2e-postgres psql -U postgres -d planeta_qosshy -Atc "$1"; }
# Kafka's command-line tools, with a small Java heap so they don't squeeze the broker.
kt() { dk exec -e KAFKA_HEAP_OPTS=-Xmx96m pq-e2e-kafka /opt/kafka/bin/"$@"; }
mp() { PYTHONIOENCODING=utf-8 python "$HERE/../e2e/mp.py" "$@"; }
# until_eq WANT SECONDS cmd args… — repeat the command until it prints WANT
until_eq() { local want=$1 n=$2; shift 2; for _ in $(seq 1 "$n"); do [ "$("$@")" = "$want" ] && return 0; sleep 1; done; return 1; }
jar() { echo "-b $W/$1.ck -c $W/$1.ck"; }
cx()  { local ip=$1; shift; curl -s -H "X-Forwarded-For: $ip" "$@"; }
loc() { curl -s -o /dev/null -w '%{redirect_url}' "$@"; }
paycard() { curl -s -o /dev/null --data-urlencode "card_number=4242 4242 4242 4242" -d 'expiry=12/30&cvv=123&holder=ALICE' "$1"; }
unpublished() { q "SELECT count(*) FROM outbox_events WHERE published_at IS NULL"; }
# How many messages the "notifier" group has not read yet (sum of LAG over partitions).
lag() { kt kafka-consumer-groups.sh --bootstrap-server localhost:29092 --describe --group notifier 2>/dev/null |
        awk '$2=="shop.order-events" && $6 ~ /^[0-9]+$/ {s+=$6} END {print s+0}'; }
# How many letters to alice have this text in the subject.
nletters() { mp alice@example.com subjects | grep -o "$1" | wc -l | tr -d ' '; }
status() { q "SELECT status FROM orders WHERE id=$1"; }

if [ "${1:-}" = down ]; then dc down -v --rmi local; exit; fi

echo "== 0. start the throw-away stand (the first build takes a few minutes)"
sed -e 's/container_name: planeta_qosshy_/container_name: pq-e2e-/' \
    -e 's/"127.0.0.1:8025:8025"/"127.0.0.1:18025:8025"/' \
    -e 's/"127.0.0.1:8085:8080"/"127.0.0.1:18085:8080"/' "$REPO/docker-compose.yml" > "$W/compose.yml"
cat > "$W/e2e.env" <<EOF
POSTGRES_PASSWORD=$(openssl rand -hex 16)
POSTGRES_HOST_PORT=55436
COOKIE_SECRET=$(openssl rand -hex 32)
SHOP_PORT=18090
BANK_PORT=18081
KAFKA_HOST_PORT=19092
PUBLIC_URL=http://localhost:18090
VERIFICATION_ADDRESS=http://localhost:18090
PAYMENT_API_KEY=$(openssl rand -hex 24)
PAYMENT_WEBHOOK_SECRET=$(openssl rand -hex 24)
SMTP_HOST=mailpit
SMTP_PORT=1025
SMTP_USER=shop@test.local
SMTP_PASS=x
TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16
EOF
# Always from a clean slate: Postgres sets POSTGRES_PASSWORD only when its volume is EMPTY,
# so a volume left by an earlier run would keep the old password and refuse the new one.
dc down -v --remove-orphans > /dev/null 2>&1
if ! dc up -d --build > "$W/up.log" 2>&1; then echo "compose up failed:"; tail -20 "$W/up.log"; exit 1; fi
check "shop answers"                        "$(curl -s -o /dev/null -w '%{http_code}' $S/healthz)" 200
check "migration 9 (outbox) applied"        "$(q "SELECT version||' '||dirty FROM schema_migrations")" "9 false"
check "kafka-init created the topic"        "$(dk inspect -f '{{.State.Status}} {{.State.ExitCode}}' pq-e2e-kafka_init)" "exited 0"
check "  ...with 3 partitions"              "$(kt kafka-topics.sh --bootstrap-server localhost:29092 --describe --topic shop.order-events | grep -o 'PartitionCount: *[0-9]*' | tr -d ' ')" "PartitionCount:3"
until_eq 1 30 sh -c "MSYS_NO_PATHCONV=1 docker logs pq-e2e-notifier 2>&1 | grep -c 'notifier: reading'"
check "notifier joined the group"           "$(dk logs pq-e2e-notifier 2>&1 | grep -c 'notifier: reading')" 1

echo "== setup: an admin, a buyer, a product"
cx 10.9.0.1 -o /dev/null -d 'username=boss&email=boss@example.com&password=supersecret1' $S/auth/register
cx 10.9.0.2 -o /dev/null -d 'username=alice&email=alice@example.com&password=supersecret1' $S/auth/register
q "UPDATE users SET verified = true; UPDATE users SET role = 'admin' WHERE email = 'boss@example.com'" >/dev/null
cx 10.9.1.1 -o /dev/null -c "$W/boss.ck"  -d 'email=boss@example.com&password=supersecret1'  $S/auth/login
cx 10.9.1.2 -o /dev/null -c "$W/alice.ck" -d 'email=alice@example.com&password=supersecret1' $S/auth/login
curl -s -o /dev/null $(jar boss) --data-urlencode "size=M:5" \
  -d 'price=12000&title=Hoodie&brand=Q&category=Hoodies&color=Red&image_url=&description=x' $S/admin/clothes
PID=$(q "SELECT id FROM clothes WHERE title='Hoodie'")
check "product created"                     "$([ -n "$PID" ] && echo yes)" yes
buy() {  # alice buys one M and pays with a test card; prints the order id
  local u; u=$(loc $(jar alice) -d 'size=M' $S/payment/$PID)
  paycard "$u"
  q "SELECT max(o.id) FROM orders o JOIN users u ON u.id = o.user_id WHERE u.email = 'alice@example.com'"
}

echo "== 1. paid → outbox → Kafka → notifier → letter"
O1=$(buy)
until_eq paid 30 status "$O1"
check "order $O1 paid"                      "$(status "$O1")" paid
until_eq 0 15 unpublished
check "outbox: 1 event, published"          "$(q "SELECT count(*)||' '||count(published_at) FROM outbox_events")" "1 1"
check "in Kafka, key = order id"            "$(kt kafka-console-consumer.sh --bootstrap-server localhost:29092 --topic shop.order-events --from-beginning --property print.key=true --timeout-ms 5000 2>/dev/null | awk '{print $1}' | sort -u | tr -d '\r\n')" "$O1"
until_eq 1 30 nletters "№$O1 оплачен"
check "letter «оплачен» in Mailpit"         "$(nletters "№$O1 оплачен")" 1
until_eq 0 15 lag
check "notifier lag"                        "$(lag)" 0

echo "== 2. notifier down: Kafka keeps the event, the notifier catches up"
dk stop pq-e2e-notifier >/dev/null
curl -s -o /dev/null $(jar boss) -d 'to=shipped' $S/admin/orders/$O1/status
until_eq 0 15 unpublished
until_eq 1 15 lag
check "«shipped» waits in Kafka (lag 1)"    "$(lag)" 1
check "  ...no letter yet"                  "$(nletters "№$O1 отправлен")" 0
dk start pq-e2e-notifier >/dev/null
until_eq 1 60 nletters "№$O1 отправлен"
check "notifier back: «отправлен» arrives"  "$(nletters "№$O1 отправлен")" 1
until_eq 0 15 lag
check "  ...lag back to 0"                  "$(lag)" 0

echo "== 3. Kafka down: the shop keeps selling, events wait in outbox_events"
dk stop pq-e2e-kafka >/dev/null
O2=$(buy)
until_eq paid 30 status "$O2"
check "order $O2 paid while Kafka is down"  "$(status "$O2")" paid
check "  ...the catalog still opens"        "$(curl -s -o /dev/null -w '%{http_code}' $S/clothes/)" 200
check "  ...the event waits in the outbox"  "$(unpublished)" 1
relay_err() { q "SELECT (max(attempts) > 0)::text || ' ' || (max(last_error) <> '')::text FROM outbox_events WHERE published_at IS NULL"; }
until_eq "true true" 40 relay_err
check "  ...relay retried, kept the error"  "$(relay_err)" "true true"
echo "      outbox row: $(q "SELECT 'attempts=' || attempts || ', last_error=' || left(last_error, 70) FROM outbox_events WHERE published_at IS NULL")"
dk start pq-e2e-kafka >/dev/null
until_eq 0 120 unpublished
check "Kafka back: the relay sent it"       "$(unpublished)" 0
check "  ...shop log: 'broker is back'"     "$([ "$(dk logs pq-e2e-api 2>&1 | grep -c 'outbox relay: broker is back')" -ge 1 ] && echo yes)" yes
until_eq 1 90 nletters "№$O2 оплачен"
check "  ...and the letter arrived"         "$(nletters "№$O2 оплачен")" 1

echo "== 4. the notifier reads the whole topic again: no duplicate letters"
dk stop pq-e2e-notifier >/dev/null
kt kafka-consumer-groups.sh --bootstrap-server localhost:29092 --group notifier --topic shop.order-events \
   --reset-offsets --to-earliest --execute >/dev/null 2>&1
check "offsets reset: all 3 events unread"  "$(lag)" 3
dk start pq-e2e-notifier >/dev/null
until_eq 0 60 lag
check "all read again (lag 0)"              "$(lag)" 0
check "  ...still one «оплачен» per order"  "$(nletters "№$O1 оплачен") $(nletters "№$O2 оплачен") $(nletters "№$O1 отправлен")" "1 1 1"
check "  ...notifier: 'already handled' ×3" "$(dk logs pq-e2e-notifier 2>&1 | grep -c 'already handled')" 3

echo "== 5. admin cancels a paid order: «отменён» and «деньги возвращены»"
printf 'нет нужного размера' > "$W/reason.txt"   # Cyrillic from a file: curl.exe mangles it in arguments
curl -s -o /dev/null $(jar boss) -d 'to=cancelled' --data-urlencode "reason@$(cygpath -m "$W/reason.txt")" $S/admin/orders/$O2/status
until_eq 1 60 nletters "№$O2 отменён"
check "letter «отменён»"                    "$(nletters "№$O2 отменён")" 1
until_eq 1 60 nletters "за заказ №$O2 возвращены"
check "letter «деньги возвращены»"          "$(nletters "за заказ №$O2 возвращены")" 1
check "order $O2 events, in order"          "$(q "SELECT string_agg(payload->>'type', ' ' ORDER BY id) FROM outbox_events WHERE msg_key = '$O2'")" "paid cancelled refund_needed refunded"
check "  ...all in ONE partition"           "$(kt kafka-console-consumer.sh --bootstrap-server localhost:29092 --topic shop.order-events --from-beginning --property print.key=true --property print.partition=true --timeout-ms 5000 2>/dev/null | awk -v k="$O2" '$2==k {print $1}' | sort -u | wc -l | tr -d ' ')" 1
check "refund_needed: no letter (internal)" "$(mp alice@example.com subjects | grep -c 'refund')" 0
check "kafka-ui answers"                    "$(until_eq 200 60 curl -s -o /dev/null -w '%{http_code}' $UI/ && echo 200)" 200

echo
echo "RESULT: $pass passed, $fail failed"
if [ "${KEEP:-0}" = 1 ]; then
  echo "The copy keeps running:  kafka-ui $UI   Mailpit http://127.0.0.1:18025   shop $S"
  echo "Delete it with:  bash scripts/kafka-demo/e2e.sh down"
else
  dc down -v --rmi local > /dev/null 2>&1 && echo "throw-away copy deleted (containers, volumes, images)"
fi
[ "$fail" = 0 ]
