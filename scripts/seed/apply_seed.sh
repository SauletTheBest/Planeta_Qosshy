#!/usr/bin/env bash
# Puts the demo catalog (made by seed_demo.py) into the LOCAL docker-compose stand.
# Run from the Planeta_Qosshy folder in Git Bash:
#   python scripts/seed/seed_demo.py /tmp/planeta-seed && bash scripts/seed/apply_seed.sh
#
# Undo:
#   docker compose exec postgres psql -U postgres -d planeta_qosshy \
#     -c "DELETE FROM clothes WHERE image_url LIKE '/uploads/clothes/seed-%'"   # sizes go with them (CASCADE)
#   docker exec planeta_qosshy_api sh -c 'rm -f /app/uploads/clothes/seed-*.svg'
set -euo pipefail
export MSYS_NO_PATHCONV=1   # Git Bash must not rewrite /app/... into C:/Program Files/Git/app/...
OUT="${1:-/tmp/planeta-seed}"

# 1. Pictures into the uploads volume. Unpacked BY THE APP USER inside the container
#    (not with docker cp, which writes as root), so the admin panel can later replace or
#    delete these files like any photo it uploaded itself.
docker exec planeta_qosshy_api mkdir -p /app/uploads/clothes
tar -C "$OUT/images" -cf - . | docker exec -i planeta_qosshy_api tar -x -C /app/uploads/clothes

# 2. Products and their sizes: one transaction (BEGIN … COMMIT inside seed.sql).
#    ON_ERROR_STOP: on the first error psql stops and the transaction is rolled back.
docker compose exec -T postgres psql -U postgres -d planeta_qosshy -v ON_ERROR_STOP=1 -q < "$OUT/seed.sql"

docker compose exec -T postgres psql -U postgres -d planeta_qosshy -Atc \
  "SELECT 'products: '||count(*)||', in the catalog: '||count(*) FILTER (WHERE in_stock AND stock > 0) FROM clothes"
