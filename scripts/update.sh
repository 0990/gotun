#!/usr/bin/env bash
# 从 GitHub 最新正式 Release 更新 Linux 二进制，并重启 gotun 服务。
set -Eeuo pipefail

usage() {
    echo "用法：sudo bash $0 [安装目录]"
    echo "安装目录默认为 /data/gotun，服务名为 gotun。"
}

if [[ ${1:-} == -h || ${1:-} == --help ]]; then
    usage
    exit 0
fi
if (( $# > 1 )); then
    usage >&2
    exit 1
fi

fail() {
    echo "[更新失败] $*" >&2
    exit 1
}

[[ $EUID -eq 0 ]] || fail "请使用 root 或 sudo 运行。"
[[ $(uname -s) == Linux ]] || fail "仅支持使用 systemd 的 Linux 系统。"
for command in curl tar sha256sum awk flock mktemp readlink cp mv chmod timeout systemctl sleep rm; do
    command -v "$command" >/dev/null 2>&1 || fail "缺少依赖：$command"
done

case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    i386 | i486 | i586 | i686) arch=386 ;;
    *) fail "不支持的架构：$(uname -m)" ;;
esac

install_dir=$(readlink -f -- "${1:-/data/gotun}")
binary="$install_dir/gotun"
backup="$install_dir/gotun.old"
[[ -f "$binary" && ! -L "$binary" ]] || fail "未找到普通二进制文件：$binary"
systemctl cat gotun >/dev/null || fail "未找到 gotun 的 systemd 服务。"

# 在安装目录加锁，避免同时更新；临时文件与二进制同盘，替换时使用原子重命名。
exec 9>"$install_dir/.gotun-update.lock"
flock -n 9 || fail "另一个更新任务正在运行。"
work_dir=$(mktemp -d "$install_dir/.gotun-update.XXXXXX")
rollback_needed=0

cleanup() {
    local status=$?
    trap - EXIT INT TERM
    set +e
    if (( rollback_needed )); then
        echo "[更新失败] 正在恢复旧二进制……" >&2
        if cp -p -- "$backup" "$work_dir/gotun.rollback" &&
            mv -fT -- "$work_dir/gotun.rollback" "$binary" &&
            timeout 60s systemctl restart gotun &&
            sleep 5 && systemctl is-active --quiet gotun; then
            echo "[已回滚] 已恢复旧二进制，gotun 服务运行正常。" >&2
        else
            echo "[回滚失败] 请检查 systemctl status gotun；旧二进制保存在 $backup" >&2
        fi
        status=1
    fi
    rm -rf -- "$work_dir"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

curl_options=(--fail --silent --show-error --location --retry 3 --connect-timeout 15 --max-time 300)
release_url=https://github.com/0990/gotun/releases
echo "[更新] 正在查询最新正式 Release……"
# 先固定版本，再从同一版本下载压缩包和校验文件，避免发布切换造成不一致。
latest_url=$(curl "${curl_options[@]}" --head --output /dev/null --write-out '%{url_effective}' "$release_url/latest")
[[ "$latest_url" == "$release_url/tag/"* ]] || fail "无法解析最新 Release：$latest_url"
version=${latest_url##*/}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(\+[0-9A-Za-z.-]+)?$ ]] || fail "最新 Release 不是正式版本：$version"
archive="gotun-${version}-linux-${arch}.tar.gz"
download_url="$release_url/download/$version"

echo "[更新] 下载 $archive"
curl "${curl_options[@]}" --output "$work_dir/$archive" "$download_url/$archive"
curl "${curl_options[@]}" --output "$work_dir/SHA256SUMS" "$download_url/SHA256SUMS"
checksum=$(awk -v name="$archive" '$2 == name {print $1}' "$work_dir/SHA256SUMS")
[[ "$checksum" =~ ^[0-9a-fA-F]{64}$ ]] || fail "SHA256SUMS 中缺少有效的 $archive 校验值。"
(cd "$work_dir" && printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check --status) || fail "下载文件的 SHA256 校验失败。"

# 只提取二进制内容，配置文件和隧道数据不会被覆盖。
tar -xzOf "$work_dir/$archive" gotun > "$work_dir/gotun.new"
[[ -s "$work_dir/gotun.new" ]] || fail "压缩包中的 gotun 二进制为空。"
chmod 755 "$work_dir/gotun.new"
binary_version=$(timeout 10s "$work_dir/gotun.new" -version)
[[ "$binary_version" == "$version" ]] || fail "二进制版本与 Release 不一致：$binary_version"
echo "[更新] 新二进制版本：$binary_version"

cp -p -- "$binary" "$work_dir/gotun.old"
mv -fT -- "$work_dir/gotun.old" "$backup"
rollback_needed=1
mv -fT -- "$work_dir/gotun.new" "$binary"
echo "[更新] 执行 systemctl restart gotun"
timeout 60s systemctl restart gotun
main_pid=$(systemctl show gotun --property=MainPID --value)
[[ "$main_pid" =~ ^[1-9][0-9]*$ ]] || fail "新版本没有运行中的主进程。"
# 等待启动后的短暂观察期，识别启动即退出或循环重启。
for ((i = 0; i < 5; i++)); do
    sleep 1
    systemctl is-active --quiet gotun || fail "新版本启动后未保持运行。"
    [[ $(systemctl show gotun --property=MainPID --value) == "$main_pid" ]] || fail "新版本启动后发生了异常重启。"
done
rollback_needed=0
echo "[更新成功] 已安装 $version（linux/$arch），gotun 服务运行正常。"
echo "[备份] $backup"
