#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
if [ -e .env ]; then
  echo '.env 已存在，未覆盖。ANE v2 需配置 PG_URI，商户设置请在 Web 面板管理。'
  exit 0
fi
umask 077
cp .env.example .env
python3 - <<'PY'
from pathlib import Path
import secrets
p=Path('.env')
p.write_text(p.read_text().replace('replace-with-at-least-16-random-characters',secrets.token_hex(16)))
PY
echo '已生成管理员密码。请填写 PG_URI 和 PUBLIC_BASE_URL 后启动。'
