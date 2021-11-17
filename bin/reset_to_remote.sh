if [[ -z "$1" ]]; then
  exit 1
fi

git fetch "$1" && git reset --hard "$1/${2:-master}"
