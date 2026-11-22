#!/usr/bin/env bash
# Gera o pacote de uma versão para instalar no servidor do box.
#
# Roda na máquina de desenvolvimento (precisa de Go e Node):
#   ./deploy/build-release.sh
# Resultado: dist/ceasa-fruit-<versão>.tar.gz
#
# O pacote leva tudo pronto (binários do Go, front compilado e estes scripts):
# o servidor não precisa de Go, nem de npm, nem de acesso ao GitHub.
set -Eeuo pipefail
cd "$(dirname "$0")/.."
ROOT=$PWD

# Versão: data + commit (ex.: 2026.11.22-1a2b3c4). Com alterações não
# commitadas, acrescenta "-dirty" para ninguém confundir com uma versão oficial.
commit=$(git rev-parse --short HEAD)
version="$(date +%Y.%m.%d)-$commit"
if [[ -n $(git status --porcelain -- api web deploy) ]]; then version="$version-dirty"; fi
name="ceasa-fruit-$version"
out="$ROOT/dist/$name"
rm -rf "$out" && mkdir -p "$out/bin"

echo "==> API (Go) -> $name/bin"
(
  cd api
  export CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH:-amd64}
  for cmd in api seed set-password; do
    go build -trimpath -ldflags "-s -w" -o "$out/bin/ceasa-$cmd" "./cmd/$cmd"
  done
)

echo "==> Front (Nuxt) -> $name/web"
(
  cd web
  npm ci --no-audit --no-fund --loglevel=error
  npm run build >/dev/null
  cp -a .output "$out/web"
)

echo "==> Scripts do servidor"
cp -a deploy "$out/deploy"
rm -f "$out/deploy/build-release.sh"
echo "$version" >"$out/VERSION"

tar -C "$ROOT/dist" -czf "$ROOT/dist/$name.tar.gz" "$name"
rm -rf "$out"
echo
echo "Pronto: dist/$name.tar.gz ($(du -h "$ROOT/dist/$name.tar.gz" | cut -f1))"
echo "No servidor: tar xzf $name.tar.gz && sudo ./$name/deploy/install.sh   (primeira vez)"
echo "             sudo ceasa-update $name.tar.gz                           (atualização)"
