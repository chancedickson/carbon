#!/bin/bash
if [ -z ${1+x} ] || [ -z ${GOPATH+x} ]; then
  echo "Usage: ./release.sh <version>"
  echo "\$GOPATH must be set!"
  exit 1
fi
rm -rf *.tar.gz
go build carbon
tar -czf "carbon $1 release.tar.gz" io.carbon.plist carbon install.sh uninstall.sh
rm carbon
