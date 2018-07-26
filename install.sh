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

 CYGWIN*|MINGW*|MSYS*)
   PLATFORM='windows'
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

curl -s -o .ek_tmp_version https://raw.githubusercontent.com/envkey/envkey-fetch/master/version.txt
VERSION=$(cat .ek_tmp_version)
rm .ek_tmp_version

welcome_envkey () {
  echo "envkey-fetch $VERSION Quick Install"
  echo "Copyright (c) 2017 Envkey Inc. - MIT License"
  echo "https://github.com/envkey/envkey-fetch"
  echo ""
}

download_envkey () {
  echo "Downloading envkey-fetch binary for ${PLATFORM}-${ARCH}"
  url="https://github.com/envkey/envkey-fetch/releases/download/v${VERSION}/envkey-fetch_${VERSION}_${PLATFORM}_${ARCH}.tar.gz"
  echo "Downloading tarball from ${url}"
  curl -s -o envkey-fetch.tar.gz "${url}"

  tar zxf envkey-fetch.tar.gz envkey-fetch.exe 2> /dev/null
  tar zxf envkey-fetch.tar.gz envkey-fetch 2> /dev/null

  if [ "$PLATFORM" == "darwin" ]; then
    mv envkey-fetch /usr/local/bin/
    echo "envkey-fetch is installed in /usr/local/bin"
  elif [ "$PLATFORM" == "windows" ]; then
    # ensure $HOME/bin exists (it's in PATH but not present in default git-bash install)
    mkdir $HOME/bin 2> /dev/null
    mv envkey-fetch.exe $HOME/bin/
    echo "envkey-fetch is installed in $HOME/bin"
  else
    sudo mv envkey-fetch /usr/local/bin/
    echo "envkey-fetch is installed in /usr/local/bin"
  fi

  rm envkey-fetch.tar.gz
  rm -f envkey-fetch
}

welcome_envkey
download_envkey

echo "Installation complete. Info:"
echo ""
envkey-fetch -h
