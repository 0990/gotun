#!/usr/bin/env bash
# 在临时目录模拟下载和 systemd，验证更新与回滚，不操作真实服务。
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo "请使用 sudo bash $0 运行。" >&2; exit 1; }
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf -- "$test_dir"' EXIT
mkdir -p "$test_dir/bin" "$test_dir/assets"
export TEST_ROOT="$test_dir"
export PATH="$test_dir/bin:$PATH"

cat > "$test_dir/bin/curl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
output=
while (( $# )); do
    case "$1" in
        --output) output=$2; shift 2 ;;
        --write-out | --retry | --connect-timeout | --max-time) shift 2 ;;
        --*) shift ;;
        *) url=$1; shift ;;
    esac
done
if [[ $url == */latest ]]; then
    printf '%s' 'https://github.com/0990/gotun/releases/tag/v1.2.3'
elif [[ $SCENARIO == download_failure ]]; then
    exit 22
else
    cp "$TEST_ROOT/assets/${url##*/}" "$output"
fi
MOCK

cat > "$test_dir/bin/systemctl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$CASE_DIR/systemctl.log"
case "$1" in
    cat) exit 0 ;;
    restart)
        count=$(cat "$CASE_DIR/restarts" 2>/dev/null || echo 0)
        echo "$((count + 1))" > "$CASE_DIR/restarts"
        [[ $SCENARIO != restart_failure || $count != 0 ]]
        ;;
    is-active)
        if [[ $SCENARIO == inactive && ! -e "$CASE_DIR/inactive_seen" ]]; then
            touch "$CASE_DIR/inactive_seen"
            exit 1
        fi
        ;;
    show)
        if [[ $SCENARIO == pid_change && -e "$CASE_DIR/pid_seen" ]]; then
            echo 101
        else
            touch "$CASE_DIR/pid_seen"
            echo 100
        fi
        ;;
    *) exit 99 ;;
esac
MOCK
printf '#!/usr/bin/env bash\nexit 0\n' > "$test_dir/bin/sleep"
chmod +x "$test_dir/bin/"*

for SCENARIO in success download_failure bad_checksum invalid_binary wrong_version restart_failure inactive pid_change locked; do
    export SCENARIO
    CASE_DIR="$test_dir/$SCENARIO"
    export CASE_DIR
    mkdir -p "$CASE_DIR/install"
    printf '原二进制\n' > "$CASE_DIR/original"
    cp "$CASE_DIR/original" "$CASE_DIR/install/gotun"
    printf '之前的备份\n' > "$CASE_DIR/install/gotun.old"
    printf '配置内容\n' > "$CASE_DIR/install/app.yaml"
    archive="gotun-v1.2.3-linux-$(uname -m).tar.gz"
    case $(uname -m) in
        x86_64) archive=gotun-v1.2.3-linux-amd64.tar.gz ;;
        aarch64) archive=gotun-v1.2.3-linux-arm64.tar.gz ;;
        i?86) archive=gotun-v1.2.3-linux-386.tar.gz ;;
    esac
    case "$SCENARIO" in
        invalid_binary) printf '#!/usr/bin/env bash\nexit 7\n' ;;
        wrong_version) printf '#!/usr/bin/env bash\necho v0.0.0\n' ;;
        *) printf '#!/usr/bin/env bash\necho v1.2.3\n' ;;
    esac > "$test_dir/assets/gotun"
    tar -czf "$test_dir/assets/$archive" -C "$test_dir/assets" gotun
    (cd "$test_dir/assets" && sha256sum "$archive" > SHA256SUMS)
    if [[ $SCENARIO == bad_checksum ]]; then
        printf '%064d  %s\n' 0 "$archive" > "$test_dir/assets/SHA256SUMS"
    fi
    if [[ $SCENARIO == locked ]]; then
        exec 8>"$CASE_DIR/install/.gotun-update.lock"
        flock -n 8
    fi
    status=0
    bash "$script_dir/update.sh" "$CASE_DIR/install" > "$CASE_DIR/output.log" 2>&1 || status=$?
    if [[ $SCENARIO == locked ]]; then
        exec 8>&-
    fi
    if [[ $SCENARIO == success ]]; then
        [[ $status == 0 ]]
        [[ $(bash "$CASE_DIR/install/gotun" -version) == v1.2.3 ]]
        cmp "$CASE_DIR/original" "$CASE_DIR/install/gotun.old"
        [[ $(cat "$CASE_DIR/restarts") == 1 ]]
    else
        [[ $status != 0 ]] || { cat "$CASE_DIR/output.log"; exit 1; }
        cmp "$CASE_DIR/original" "$CASE_DIR/install/gotun"
        case "$SCENARIO" in
            restart_failure | inactive | pid_change)
                [[ $(cat "$CASE_DIR/restarts") == 2 ]]
                cmp "$CASE_DIR/original" "$CASE_DIR/install/gotun.old"
                ;;
            *)
                [[ ! -e "$CASE_DIR/restarts" ]]
                [[ $(cat "$CASE_DIR/install/gotun.old") == 之前的备份 ]]
                ;;
        esac
    fi
    [[ $(cat "$CASE_DIR/install/app.yaml") == 配置内容 ]]
    ! compgen -G "$CASE_DIR/install/.gotun-update.??????" >/dev/null
    echo "通过：$SCENARIO"
done
