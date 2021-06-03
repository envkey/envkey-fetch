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
elif [[ "$(uname -m)" == armv5* ]]; then
  ARCH="armv5"
elif [[ "$(uname -m)" == armv6* ]]; then
  ARCH="armv6"
elif [[ "$(uname -m)" == armv7* ]]; then
  ARCH="armv7"
elif [[ "$(uname -m)" == 'arm64' ]]; then
  ARCH="arm64"
elif [[ "$(uname -m)" == 'aarch64' ]]; then
  ARCH="arm64"
else
  ARCH="386"
fi

if [[ "$(cat /proc/1/cgroup 2> /dev/null | grep docker | wc -l)" > 0 ]] || [ -f /.dockerenv ]; then
  IS_DOCKER=true
else
  IS_DOCKER=false
fi

curl -s -o .ek_tmp_version https://raw.githubusercontent.com/envkey/envkey-fetch/master/version.txt
VERSION=$(cat .ek_tmp_version)
rm .ek_tmp_version

welcome_envkey () {
  echo "envkey-fetch $VERSION Quick Install"
  echo "Copyright (c) 2021 Envkey Inc. - MIT License"
  echo "https://github.com/envkey/envkey-fetch"
  echo ""
}

cleanup () {
  rm envkey-fetch.tar.gz
  rm -f envkey-fetch
}

download_envkey () {
  echo "Downloading envkey-fetch binary for ${PLATFORM}-${ARCH}"
  url="https://github.com/envkey/envkey-fetch/releases/download/v${VERSION}/envkey-fetch_${VERSION}_${PLATFORM}_${ARCH}.tar.gz"
  echo "Downloading tarball from ${url}"
  curl -s -L -o envkey-fetch.tar.gz "${url}"

  tar zxf envkey-fetch.tar.gz envkey-fetch.exe 2> /dev/null
  tar zxf envkey-fetch.tar.gz envkey-fetch 2> /dev/null

  if [ "$PLATFORM" == "darwin" ] || $IS_DOCKER ; then
    if [[ -d /usr/local/bin ]]; then
      mv envkey-fetch /usr/local/bin/
      echo "envkey-fetch is installed in /usr/local/bin"
    else
      echo >&2 'Error: /usr/local/bin does not exist. Create this directory with appropriate permissions, then re-install.'
      cleanup
      exit 1
    fi
  elif [ "$PLATFORM" == "windows" ]; then
    # ensure $HOME/bin exists (it's in PATH but not present in default git-bash install)
    mkdir $HOME/bin 2> /dev/null
    mv envkey-fetch.exe $HOME/bin/
    echo "envkey-fetch is installed in $HOME/bin"
  else
    sudo mv envkey-fetch /usr/local/bin/
    echo "envkey-fetch is installed in /usr/local/bin"
  fi
}

welcome_envkey
download_envkey
cleanup

echo "Installation complete. Info:"
echo ""
envkey-fetch -h
