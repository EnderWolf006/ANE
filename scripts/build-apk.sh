#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/../android"
umask 077
if [ ! -e keystore.properties ]; then
  if [ -e keystore/ane-release.jks ]; then
    echo '签名文件已存在，但缺少 keystore.properties。请恢复密码配置，禁止覆盖旧密钥。' >&2
    exit 1
  fi
  mkdir -p keystore
  ANE_SIGNING_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(24))')
  export ANE_SIGNING_PASSWORD
  keytool -genkeypair -keystore keystore/ane-release.jks -storetype PKCS12 \
    -alias ane -keyalg RSA -keysize 3072 -validity 10000 \
    -storepass:env ANE_SIGNING_PASSWORD -keypass:env ANE_SIGNING_PASSWORD \
    -dname 'CN=ANE, OU=ANE, O=ANE, C=CN'
  cat > keystore.properties <<PROPERTIES
storeFile=keystore/ane-release.jks
storePassword=$ANE_SIGNING_PASSWORD
keyAlias=ane
keyPassword=$ANE_SIGNING_PASSWORD
PROPERTIES
  unset ANE_SIGNING_PASSWORD
  echo '已创建本地签名。请备份 android/keystore 与 android/keystore.properties，后续更新必须使用同一签名。'
fi
./gradlew --no-daemon "$@" :app:assembleRelease :app:testDebugUnitTest :app:lintRelease
mkdir -p ../dist
cp app/build/outputs/apk/release/app-release.apk ../dist/ANE.apk
cd ../dist
if command -v sha256sum >/dev/null 2>&1; then sha256sum ANE.apk > ANE.apk.sha256; else shasum -a 256 ANE.apk > ANE.apk.sha256; fi
echo '已生成 dist/ANE.apk 与校验文件。'
