# !/usr/bin/env bash

case "$(uname -s)" in
 Darwin)
   PLATFORM='darwin'
   ;;

 Linux)
   PLATFORM='linux'
   ;;

 FreeBSD)
   PLATFORM='freebsd'
   ;;

 CYGWIN*|MINGW32*|MSYS*)
   echo "This install script is not compatible with windows. Download the latest .exe release and put it in your PATH: https://github.com/envkey/envkey-fetch/releases"
   exit 1
   ;;

 *)
   echo "Platform may or may not be supported. Will attempt to install."
   PLATFORM='linux'
   ;;
esac

if [[ "$(uname -m)" == 'x86_64' ]]; then
  ARCH="amd64"
else
  ARCH="386"
fi

curl -s -o version.txt https://raw.githubusercontent.com/envkey/envkey-fetch/master/version.txt
VERSION=$(cat version.txt)
rm version.txt

function welcome_envkey {
  echo "envkey-fetch $VERSION Quick Install"
  echo "Copyright (c) 2017 Envkey Inc. - MIT License"
  echo "https://github.com/envkey/envkey-fetch"
  echo ""
}

function download_envkey {
  echo "Downloading envkey-fetch binary for ${PLATFORM}-${ARCH}"
  url="https://raw.githubusercontent.com/envkey/envkey-fetch/master/dist/envkey-fetch_${VERSION}_${PLATFORM}_${ARCH}.tar.gz"
  echo "Downloading tarball from ${url}"
  curl -s -o envkey-fetch.tar.gz "${url}"
  tar zxf envkey-fetch.tar.gz

  if [["$PLATFORM" == 'darwin']]; then
    mv envkey-fetch /usr/local/bin/
  else
    sudo envkey-fetch /usr/local/bin/
  fi

  echo "envkey-fetch is installed in /usr/local/bin"

  rm envkey-fetch.tar.gz
  rm -f envkey-fetch
}

welcome_envkey
download_envkey

echo "Installation complete. Info:"
echo ""
/usr/local/bin/envkey-fetch -h
