// 使用真实启动包与动态模板，在内存接口上验证界面，避免修改运行实例的配置。
// npm install --prefix <临时目录> playwright
// 设置 PLAYWRIGHT_MODULE_PATH 为该目录下 node_modules/playwright 的路径后运行本脚本。
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH || "playwright");

const root = path.resolve(__dirname, "..");
const output = process.env.ADMIN_UI_SCREENSHOTS;
const tunnels = [
    {
        uuid: "a",
        name: "goai_tcpmux_pw",
        disabled: false,
        status: "running",
        mode: "default",
        input: "socks5x@0.0.0.0:1072",
        output: "tcp_mux@154.36.173.131:4000",
        quality_summary: { status: "up", rtt_ms: 159, loss_pct: 0 },
        bandwidth_summary: { status: "disabled" },
        in_proto_cfg: "{}",
        out_proto_cfg: "{}",
        in_decrypt_mode: "none",
        out_crypt_mode: "none",
    },
    {
        uuid: "b",
        name: "备用线路与很长的名称_for_mobile_ipv6_layout",
        disabled: true,
        status: "disabled",
        input: "tcp@[2001:db8:1234:5678:abcd:1234:5678:abcd]:1080",
        output: "quic@[2001:db8:1234:5678:abcd:1234:5678:abcd]:4000",
        quality_summary: { status: "disabled" },
    },
    {
        uuid: "c",
        name: "failed_tunnel",
        disabled: false,
        status: "input run:bind failed",
        input: "udp@0.0.0.0:1081",
        output: "udp@127.0.0.1:4000",
        quality_summary: { status: "down", rtt_ms: -1, loss_pct: 100 },
    },
];
const routeFixture = (id) => ({
    uuid: id,
    name: "auto_" + id,
    disabled: false,
    status: "running",
    listen: "socks5x@[2001:db8::1]:1088",
    members: [tunnels[0].name, tunnels[1].name],
    preferred_member: tunnels[0].name,
    member_health: { [tunnels[0].name]: "up", [tunnels[1].name]: "unknown" },
});
const backupMemberName = tunnels[1].name;
const records = {
    tunnel: tunnels,
    route_output: [routeFixture("output")],
    route_input: [{ ...routeFixture("input"), listen: "[2001:db8::1]:1088", policy: "quality",
        quality: { download_url: "https://speed.cloudflare.com/__down?bytes=16384", connectivity_url: "https://www.gstatic.com/generate_204", download_status: 200, connectivity_status: 204, download_bytes: 16384, interval_sec: 30, timeout_ms: 5000, normal_ms: 2000 },
        member_quality: { [tunnels[0].name]: { status: "normal", samples: 12, bad_samples: 2, consecutive_failures: 0, last_duration_ms: 800 }, [tunnels[1].name]: { status: "failed", samples: 12, bad_samples: 4, consecutive_failures: 2, last_duration_ms: 5000, last_error: "完整路径探测超时，需要检查代理协商以及最终下载目标是否可达" } },
    }],
    service: ["echo", "http_proxy", "socks5_server", "socks5x_server"].map((type, i) => ({
        uuid: "s" + i,
        name: type,
        disabled: false,
        status: "running",
        type,
        listen: "127.0.0.1:4003",
        listen_port: 1085,
        tcp_timeout: 300,
        udp_timeout: 120,
    })),
};
records.route_input[0].probe_mode = "full_path";
records.route_input.push({
    ...structuredClone(records.route_input[0]), uuid: "priority", name: "auto_priority", policy: "priority",
    quality: { ...records.route_input[0].quality, download_url: "https://priority.example.com/probe.bin", interval_sec: 45 },
    member_quality: {
        [tunnels[0].name]: { status: "normal", samples: 2, bad_samples: 0, consecutive_failures: 0, consecutive_normal: 2, last_duration_ms: 800 },
        [tunnels[1].name]: { status: "failed", samples: 2, bad_samples: 2, consecutive_failures: 2, consecutive_normal: 0, last_duration_ms: 5000 },
    },
});
records.route_input.push({
    ...routeFixture("legacy"), listen: "127.0.0.1:1089", policy: "priority", probe_mode: "legacy_frame",
    probe_warnings: ["legacy_tcp：完整链路探测要求 member 的 input 为 socks5x"],
});
let failSave = false,
    failList = false,
    failToggle = false,
    probeCount = 0,
    streamClosed = false;
const writes = [];
let failHistory = false;
const historyReads = [];
const historyTime = new Date(Math.floor((Date.now() - 5 * 60000) / 1000) * 1000 + 8 * 3600000).toISOString().slice(0, 19);
const historyFixture = Array.from({ length: 55 }, (_, i) => ({
    id: i + 1, tunnel_uuid: "a", time: historyTime + ".123+08:00",
    kind: i % 2 ? "route" : "frame", route_name: i % 2 ? "socks5-password" : "",
    status: i % 2 ? "failed" : "normal", duration_ms: i % 2 ? 5000 : 159,
    connectivity: i % 2 ? "ok" : "", error: i % 2 ? "下载探测超时" : "",
}));

function json(res, data) {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(data));
}
const server = http.createServer(async (req, res) => {
    try {
        const url = new URL(req.url, "http://localhost");
        if (url.pathname.startsWith("/api/")) {
            let body = "";
            for await (const chunk of req) body += chunk;
            let payload = body ? JSON.parse(body) : {};
            if (url.pathname === "/api/auth/session")
                return json(res, { code: 200, data: { authenticated: true, username: "admin" } });
            if (url.pathname === "/api/tunnel/probe") {
                probeCount++;
                return json(res, { code: 200 });
            }
            if (url.pathname === "/api/tunnel/mtr/stream") {
                res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-cache" });
                res.write(
                    "event: start\ndata: " +
                        JSON.stringify({ name: tunnels[0].name, command: "mtr --tcp 127.0.0.1" }) +
                        "\n\n",
                );
                res.write(
                    "event: terminal\ndata: " +
                        JSON.stringify({
                            chunk: Buffer.from("Host  Loss%  Last\r\nlocalhost  0%  1ms\r\n").toString(
                                "base64",
                            ),
                        }) +
                        "\n\n",
                );
                res.on("close", () => {
                    streamClosed = true;
                });
                return;
            }
            const [, , kind, action] = url.pathname.split("/");
            if (action === "list") {
                if (failList) return json(res, { code: 500, msg: "列表加载失败测试" });
                return json(res, {
                    code: 200,
                    data: {
                        version: "ui-test",
                        list: records[kind],
                        pagination: {
                            total_nums: records[kind].length,
                            page_size: 20,
                            current_page: Number(url.searchParams.get("page") || 1),
                        },
                    },
                });
            }
            if (action === "quality")
                return json(res, { code: 200, data: { summary: tunnels[0].quality_summary, details: {} } });
            if (action === "probe_history") {
                historyReads.push(Object.fromEntries(url.searchParams));
                if (failHistory) return json(res, { code: 500, msg: "历史查询失败测试" });
                const before = Number(url.searchParams.get("before") || Infinity);
                const since = Number(url.searchParams.get("since")), until = Number(url.searchParams.get("until"));
                const kind = url.searchParams.get("kind"), status = url.searchParams.get("status");
                const matches = historyFixture.filter(row => row.tunnel_uuid === url.searchParams.get("uuid") && row.id < before && Date.parse(row.time) >= since && Date.parse(row.time) <= until && (!kind || row.kind === kind) && (!status || row.status === status)).reverse();
                const rows = matches.slice(0, 50);
                return json(res, { code: 200, data: { records: rows, next_before: matches.length > 50 ? rows.at(-1).id : 0 } });
            }
            if (action === "switch_log")
                return json(res, {
                    code: 200,
                    data: [{ time: "2026-10-17 18:10:00", from: tunnels[1].name, to: tunnels[0].name }],
                });
            if (action === "check_server") return json(res, { code: 200, msg: "检测完成（内存接口）" });
            if (action === "bandwidth")
                return json(res, { code: 200, data: { summary: { status: "ok", mbps: 50 } } });
            writes.push({ kind, action, payload, url: url.toString() });
            if (action === "set_disabled") {
                if (failToggle) return json(res, { code: 500, msg: "启停失败测试" });
                const name = url.searchParams.get("name") || payload.name;
                const row = records[kind].find((r) => r.name === name);
                row.disabled = url.searchParams.has("disabled")
                    ? url.searchParams.get("disabled") === "true"
                    : payload.disabled;
                row.status = row.disabled ? "disabled" : "running";
            }
            if (["create", "edit", "import"].includes(action)) {
                if (failSave) return json(res, { code: 500, msg: "保存失败测试" });
                if (action === "edit")
                    Object.assign(
                        records[kind].find((r) => r.uuid === payload.uuid),
                        payload,
                    );
                else
                    records[kind].unshift({
                        ...payload,
                        uuid: payload.uuid || "created",
                        status: payload.disabled ? "disabled" : "running",
                    });
            }
            if (action === "delete") {
                records[kind] = records[kind].filter((r) =>
                    payload.uuid ? r.uuid !== payload.uuid : r.name !== payload.name,
                );
            }
            return json(res, { code: 200 });
        }
        let file;
        if (url.pathname === "/render") {
            const component = url.searchParams.get("path");
            assert.match(
                component,
                /^\/(layout\/default|tunnel\/(list|create|edit)|route_(input|output)\/list|service\/list)$/,
            );
            file = path.join(root, "admin/view", component + ".html");
        } else if (url.pathname === "/login") file = path.join(root, "admin/view/login.html");
        else if (url.pathname.startsWith("/go_sword_public/"))
            file = path.join(root, url.pathname.slice("/go_sword_public/".length));
        else
            file = path.join(root, "admin/resource/dist", url.pathname === "/" ? "index.html" : url.pathname);
        const mime = {
            ".js": "text/javascript",
            ".css": "text/css",
            ".html": "text/html",
            ".woff2": "font/woff2",
            ".woff": "font/woff",
        };
        res.writeHead(200, { "Content-Type": mime[path.extname(file)] || "application/octet-stream" });
        res.end(fs.readFileSync(file));
    } catch (error) {
        res.writeHead(500);
        res.end(String(error));
    }
});

async function main() {
    await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
    const base = "http://127.0.0.1:" + server.address().port;
    const browser = await chromium.launch({
        channel: process.env.PLAYWRIGHT_CHANNEL || "chrome",
        headless: true,
    });
    const page = await browser.newPage({ viewport: { width: 1440, height: 1050 }, timezoneId: "America/Los_Angeles" });
    page.setDefaultTimeout(8000);
    const errors = [];
    page.on("pageerror", (error) => errors.push(String(error)));
    page.on("console", (msg) => {
        if (msg.type() === "error") errors.push(msg.text());
    });
    const screenshot = async (name) => {
        if (output) {
            await page.waitForTimeout(400);
            fs.mkdirSync(output, { recursive: true });
            await page.screenshot({
                path: path.join(output, name + ".png"),
                fullPage: !name.includes("mtr"),
            });
        }
    };
    const overflow = async () =>
        assert.equal(
            await page.evaluate(() => document.documentElement.scrollWidth > innerWidth),
            false,
            "整页横向溢出",
        );
    const readableText = async () => {
        const failures = await page.evaluate(() => {
            const luminance = (rgb) =>
                rgb
                    .slice(0, 3)
                    .map((value) => {
                        const channel = value / 255;
                        return channel <= 0.04045
                            ? channel / 12.92
                            : Math.pow((channel + 0.055) / 1.055, 2.4);
                    })
                    .reduce((total, value, index) => total + value * [0.2126, 0.7152, 0.0722][index], 0);
            const rgb = (color) => (color.match(/[\d.]+/g) || []).map(Number);
            const selectors =
                ".g-version,.g-nav-caption,.g-nav-link,.g-page-heading p,.g-summary>div>span,.g-refresh,.g-segmented button,.g-desktop-table th,.g-name,.g-subtext,.g-proto,.g-endpoint,.g-quality small,.g-page-note,.g-member-name,.g-form-note,.g-detail-key,.g-field>span,.g-advanced summary,.g-bottom-nav button,.ivu-form-item-label,.ivu-select-placeholder";
            return [...document.querySelectorAll(selectors)]
                .filter((el) => el.getClientRects().length && el.textContent.trim())
                .flatMap((el) => {
                    let background = [255, 255, 255];
                    for (let parent = el; parent; parent = parent.parentElement) {
                        const candidate = rgb(getComputedStyle(parent).backgroundColor);
                        if (candidate.length === 3 || candidate[3] > 0) {
                            background = candidate;
                            break;
                        }
                    }
                    const textLuminance = luminance(rgb(getComputedStyle(el).color));
                    const backgroundLuminance = luminance(background);
                    const ratio =
                        (Math.max(textLuminance, backgroundLuminance) + 0.05) /
                        (Math.min(textLuminance, backgroundLuminance) + 0.05);
                    return ratio < 4.5
                        ? [{ text: el.textContent.trim().slice(0, 50), ratio: Number(ratio.toFixed(2)) }]
                        : [];
                });
        });
        assert.deepEqual(failures, [], "可读文字与背景的对比度不足");
    };
    const goto = async (kind) => {
        await page.goto(base + "/#/" + kind + "/list");
        await page.locator(".g-page-heading").waitFor();
        await page.waitForFunction(() => !document.body.innerText.includes("正在加载列表"));
    };
    const modal = () => page.locator(".ivu-modal:visible");
    const save = () => modal().locator(".ivu-modal-footer").getByText("保存配置", { exact: true });
    try {
        await goto("tunnel");
        await page.locator(".g-desktop-table tbody tr").first().waitFor();
        assert.equal(await page.locator(".g-summary>div").nth(1).locator("strong").textContent(), "1");
        assert.equal(await page.locator(".g-desktop-table tbody tr").count(), 1, "隧道管理默认仅显示运行中");
        await page.locator(".g-desktop-table .g-more").first().click();
        await page.locator(".ivu-dropdown-transfer:visible").getByText("探测历史", { exact: true }).click();
        await modal().getByText("探测历史 - goai_tcpmux_pw", { exact: true }).waitFor();
        await modal().getByText(historyTime.replace("T", " "), { exact: true }).first().waitFor();
        assert.equal(await modal().locator(".ivu-table-tbody tr").count(), 50);
        assert.equal(historyReads.at(-1).uuid, "a");
        await modal().getByRole("button", { name: "下一页", exact: true }).click();
        await modal().getByText("第 2 页", { exact: true }).waitFor();
        await page.waitForFunction(() => document.querySelectorAll(".ivu-modal:not([style*='display: none']) .ivu-table-tbody tr").length === 5);
        assert.equal(historyReads.at(-1).before, "6");
        await modal().getByRole("button", { name: "上一页", exact: true }).click();
        await modal().getByText("第 1 页", { exact: true }).waitFor();
        await modal().getByLabel("探测类型").selectOption("route");
        await modal().getByText("入口：socks5-password", { exact: true }).first().waitFor();
        assert.equal(historyReads.at(-1).kind, "route");
        await modal().getByLabel("探测结果").selectOption("slow");
        await modal().getByText("此范围内暂无探测记录", { exact: true }).waitFor();
        assert.equal(historyReads.at(-1).status, "slow");
        await modal().getByLabel("时间范围").selectOption("168");
        await page.waitForFunction(() => document.querySelector(".g-history-filters select").value === "168");
        await page.waitForTimeout(100);
        assert.equal(Number(historyReads.at(-1).until) - Number(historyReads.at(-1).since), 168 * 3600000);
        failHistory = true;
        await modal().getByRole("button", { name: "刷新", exact: true }).click();
        await modal().getByRole("alert").getByText("历史查询失败测试", { exact: false }).waitFor();
        failHistory = false;
        await modal().getByRole("button", { name: "重试", exact: true }).click();
        await modal().getByText("此范围内暂无探测记录", { exact: true }).waitFor();
        await modal().getByLabel("探测结果").selectOption("");
        await modal().getByText("入口：socks5-password", { exact: true }).first().waitFor();
        const historyQuery = async (action) => {
            const response = page.waitForResponse(res => res.url().includes("/api/tunnel/probe_history?"));
            await Promise.all([response, action()]);
            await page.waitForFunction(() => document.querySelector(".g-history-table").getAttribute("aria-busy") === "false");
        };
        await historyQuery(() => modal().getByLabel("探测类型").selectOption(""));
        const preset = historyReads.at(-1);
        await historyQuery(() => modal().getByLabel("时间范围").selectOption("custom"));
        const beijingInput = ms => new Date(Number(ms) + 8 * 3600000).toISOString().slice(0, 19);
        const startInput = modal().getByLabel("开始时间（北京时间）", { exact: true });
        const endInput = modal().getByLabel("结束时间（北京时间）", { exact: true });
        assert.equal(await startInput.inputValue(), beijingInput(preset.since), "自定义初值应使用北京时间");
        assert.equal(await endInput.inputValue(), beijingInput(preset.until));
        const changeTime = async (input, value) => {
            await input.fill(value);
            await input.dispatchEvent("change");
        };
        await historyQuery(() => changeTime(startInput, historyTime));
        await historyQuery(() => changeTime(endInput, historyTime));
        assert.equal(await modal().locator(".ivu-table-tbody tr").count(), 50, "相同起止秒应包含该秒内的记录");
        const custom = { ...historyReads.at(-1) };
        assert.equal(Number(custom.since), Date.parse(historyTime + "+08:00"), "查询时间不能随浏览器时区变化");
        assert.equal(Number(custom.until), Number(custom.since) + 999);
        await historyQuery(() => modal().getByRole("button", { name: "下一页", exact: true }).click());
        assert.equal(await modal().locator(".ivu-table-tbody tr").count(), 5);
        assert.equal(historyReads.at(-1).since, custom.since);
        assert.equal(historyReads.at(-1).until, custom.until);
        await historyQuery(() => modal().getByLabel("探测类型").selectOption("route"));
        await modal().getByText("第 1 页", { exact: true }).waitFor();
        assert.equal(historyReads.at(-1).before, undefined, "修改筛选应重置游标");
        assert.equal(historyReads.at(-1).since, custom.since);
        await historyQuery(() => modal().getByRole("button", { name: "刷新", exact: true }).click());
        assert.equal(historyReads.at(-1).until, custom.until, "刷新应保留自定义区间");
        const readCount = historyReads.length;
        await changeTime(startInput, beijingInput(Number(custom.until) + 1000));
        await modal().getByRole("alert").getByText("开始时间不能晚于结束时间", { exact: false }).waitFor();
        await changeTime(startInput, "");
        await modal().getByRole("alert").getByText("请选择完整的开始时间和结束时间", { exact: false }).waitFor();
        assert.equal(historyReads.length, readCount, "无效时间区间不能发送查询");
        await historyQuery(() => changeTime(startInput, historyTime));
        const beforeRecord = beijingInput(Number(custom.since) - 1000);
        await historyQuery(() => changeTime(startInput, beforeRecord));
        await historyQuery(() => changeTime(endInput, beforeRecord));
        await modal().getByText("此范围内暂无探测记录", { exact: true }).waitFor();
        const afterRecord = beijingInput(Number(custom.until) + 1000);
        await historyQuery(() => changeTime(endInput, afterRecord));
        await historyQuery(() => changeTime(startInput, afterRecord));
        await modal().getByText("此范围内暂无探测记录", { exact: true }).waitFor();
        await historyQuery(() => changeTime(startInput, historyTime));
        await historyQuery(() => changeTime(endInput, historyTime));
        await historyQuery(() => modal().getByLabel("时间范围").selectOption("1"));
        assert.equal(await startInput.count(), 0);
        assert.equal(Number(historyReads.at(-1).until) - Number(historyReads.at(-1).since), 3600000);
        await historyQuery(() => modal().getByLabel("时间范围").selectOption("custom"));
        assert.equal(historyReads.at(-1).since, custom.since, "切换回自定义应保留所选区间");
        await screenshot("desktop-history");
        await page.setViewportSize({ width: 390, height: 844 });
        await overflow();
        await screenshot("mobile-history");
        await modal().locator(".ivu-modal-close").click();
        await page.setViewportSize({ width: 1440, height: 1050 });
        await screenshot("desktop");
        assert.ok(
            await page
                .locator(".g-desktop-table .g-endpoint")
                .first()
                .evaluate((el) => parseFloat(getComputedStyle(el).fontSize) >= 14),
        );
        for (const width of [320, 375, 390, 414, 768, 900, 1024, 1440, 1920]) {
            await page.setViewportSize({ width, height: 844 });
            await overflow();
            await readableText();
        }
        await page.setViewportSize({ width: 390, height: 844 });
        assert.ok(
            await page
                .locator(".g-card-endpoint .g-endpoint")
                .first()
                .evaluate((el) => parseFloat(getComputedStyle(el).fontSize) >= 13),
        );
        for (const label of await page.locator(".g-endpoint-label").evaluateAll((els) =>
            els.map((el) => ({
                width: el.clientWidth,
                height: el.clientHeight,
                font: parseFloat(getComputedStyle(el).fontSize),
            })),
        )) {
            assert.ok(label.width >= label.font * 2 && label.height < label.font * 2.5, "入口出口标签换行");
        }
        await screenshot("mobile");
        await page.getByRole("button", { name: "全部 3", exact: true }).click();
        await page.getByRole("searchbox").fill("quic");
        assert.equal(await page.locator(".g-card").count(), 1);
        await page.getByRole("searchbox").fill("");
        await page.getByRole("button", { name: "已停用 1", exact: true }).click();
        assert.equal(await page.locator(".g-card").count(), 1);
        await page.getByRole("button", { name: "全部 3", exact: true }).click();
        failToggle = true;
        await page.locator(".g-card .g-switch").first().click();
        await page.getByText("启停失败测试", { exact: true }).waitFor();
        assert.equal(await page.locator(".g-card .g-switch").first().getAttribute("aria-checked"), "true");
        failToggle = false;
        await page.locator(".g-card .g-switch").first().click();
        await page.waitForFunction(
            () => document.querySelector(".g-card .g-switch").getAttribute("aria-checked") === "false",
        );
        await page.locator(".g-card .g-switch").first().click();
        await page.waitForFunction(
            () => document.querySelector(".g-card .g-switch").getAttribute("aria-checked") === "true",
        );
        await page.locator(".g-card").first().getByText("编辑", { exact: true }).click();
        await page.locator(".g-form-panel").waitFor();
        await page.reload();
        await page.locator(".g-form-panel").waitFor();
        await page.locator(".g-field input").first().fill("edited_tunnel");
        await page.locator(".g-advanced summary").click();
        assert.equal(await page.locator(".g-advanced textarea").count(), 4);
        await overflow();
        await readableText();
        failSave = true;
        await page.getByRole("button", { name: "保存配置", exact: true }).click();
        await page.getByText("保存失败测试", { exact: true }).waitFor();
        assert.ok(await page.locator(".g-form-panel").isVisible());
        failSave = false;
        await page.getByRole("button", { name: "保存配置", exact: true }).click();
        await page.locator(".g-card").first().getByText("edited_tunnel", { exact: true }).waitFor();
        const edit = writes.find((w) => w.kind === "tunnel" && w.action === "edit");
        for (const key of [
            "input",
            "output",
            "mode",
            "in_proto_cfg",
            "out_proto_cfg",
            "in_decrypt_mode",
            "in_decrypt_key",
            "out_crypt_mode",
            "out_crypt_key",
            "in_extend",
            "out_extend",
        ])
            assert.ok(key in edit.payload, key);
        await page.locator(".g-card .g-more").first().click();
        await page.locator(".ivu-dropdown-transfer:visible").getByText("MTR 诊断", { exact: true }).click();
        await page.locator(".xterm-screen").waitFor();
        await overflow();
        await screenshot("mobile-mtr");
        await page.locator(".ivu-modal-footer:visible").getByText("关闭", { exact: true }).click();
        await page.locator(".mtr-modal-shell .ivu-modal").waitFor({ state: "hidden" });
        await new Promise((resolve) => setTimeout(resolve, 100));
        assert.ok(streamClosed, "关闭 MTR 后未释放连接");
        // 克隆与导入继续使用现有接口，克隆必须默认禁用并保留原协议参数。
        await page.locator(".g-card .g-more").first().click();
        await page.locator(".ivu-dropdown-transfer:visible").getByText("克隆隧道", { exact: true }).click();
        await page.getByRole("button", { name: /^全部 \d+$/ }).click();
        await page.locator(".g-card").getByText("edited_tunnel-copy", { exact: true }).waitFor();
        const clone = writes.filter((w) => w.kind === "tunnel" && w.action === "import").at(-1).payload;
        assert.equal(clone.disabled, true);
        assert.equal(clone.input, tunnels.find((t) => t.name === "edited_tunnel").input);
        assert.notEqual(clone.uuid, tunnels.find((t) => t.name === "edited_tunnel").uuid);
        await page.locator("#input").setInputFiles({
            name: "browser-import.tun",
            mimeType: "application/json",
            buffer: Buffer.from(JSON.stringify({ ...clone, uuid: "browser-import", name: "browser-import" })),
        });
        await page.locator(".g-card").getByText("browser-import", { exact: true }).waitFor();
        await page.locator(".g-card .g-more").first().click();
        const [exportPage] = await Promise.all([
            page.waitForEvent("popup"),
            page.locator(".ivu-dropdown-transfer:visible").getByText("导出配置", { exact: true }).click(),
        ]);
        await exportPage.waitForLoadState();
        assert.ok(exportPage.url().includes("/api/tunnel/export?name=browser-import"));
        await exportPage.close();
        await page.locator(".g-card .g-more").first().click();
        await page.locator(".ivu-dropdown-transfer:visible").getByText("删除隧道", { exact: true }).click();
        await modal().getByRole("button", { name: "确认删除", exact: true }).click();
        await page
            .locator(".g-card")
            .getByText("browser-import", { exact: true })
            .waitFor({ state: "hidden" });
        await page.getByRole("button", { name: "检测工具", exact: false }).click();
        await modal().getByRole("button", { name: "SOCKS5 检测", exact: true }).click();
        await page.getByText("socks5 tcp 检测结果", { exact: true }).waitFor();
        await page.getByText("socks5 udp 检测结果", { exact: true }).waitFor();
        await modal().locator(".ivu-switch").click();
        await page.waitForFunction(() => document.body.innerText.includes("已开启，每秒探测"));
        await modal().locator(".ivu-modal-close").click();
        await page.locator(".g-bottom-nav").getByText("路由出口", { exact: true }).click();
        await page.locator(".g-page-heading h1").getByText("智能路由 · 出口", { exact: true }).waitFor();
        const count = probeCount;
        await new Promise((resolve) => setTimeout(resolve, 1100));
        assert.equal(probeCount, count, "离开页面后探测未停止");
        for (const [kind, label] of [
            ["route_output", "路由出口"],
            ["route_input", "路由入口"],
            ["service", "服务"],
        ]) {
            await page.locator(".g-bottom-nav").getByText(label, { exact: true }).click();
            await page.locator(".g-card").first().waitFor();
            await overflow();
            await readableText();
            await page.locator(".g-card").first().getByText("编辑", { exact: true }).click();
            await modal().waitFor();
            await overflow();
            await readableText();
            if (kind !== "service") {
                await modal()
                    .locator(".g-member-order li")
                    .first()
                    .getByRole("button", { name: /下移/ })
                    .click();
                assert.ok(
                    (await modal().locator(".g-member-order li").first().textContent()).includes(
                        backupMemberName,
                    ),
                );
            }
            failSave = true;
            if (kind === "route_input") {
                await modal().getByText("2 条共 2.83 GB", { exact: false }).waitFor();
                await modal().getByPlaceholder("HTTPS 下载 URL").fill("https://download.example.com/probe.bin");
                await modal().getByPlaceholder("HTTPS 连通性 URL").fill("https://check.example.com/ok");
                const interval = modal().locator('[aria-label="探测间隔秒"] input');
                await interval.fill("60");
                await interval.blur();
                await modal().getByText("2 条共 1.42 GB", { exact: false }).waitFor();
                await overflow();
                await readableText();
                await screenshot("mobile-quality-config");
            }
            const [failedSave] = await Promise.all([
                page.waitForResponse((response) => response.url().includes("/api/" + kind + "/edit")),
                save().click(),
            ]);
            assert.equal((await failedSave.json()).code, 500);
            await page.getByText("保存失败测试", { exact: true }).last().waitFor();
            assert.ok(await modal().isVisible());
            failSave = false;
            await save().click();
            await modal().waitFor({ state: "hidden" });
            if (kind !== "service")
                assert.equal(
                    writes.filter((w) => w.kind === kind && w.action === "edit").at(-1).payload.members[0],
                    backupMemberName,
                );
            if (kind === "route_input") {
                const payload = writes.filter((w) => w.kind === kind && w.action === "edit").at(-1).payload;
                assert.equal(payload.policy, "quality");
                assert.equal(payload.quality.download_url, "https://download.example.com/probe.bin");
                assert.equal(payload.quality.connectivity_url, "https://check.example.com/ok");
                assert.equal(payload.quality.interval_sec, 60);
                await page.locator(".g-card").first().getByText("编辑", { exact: true }).click();
                await modal().waitFor();
                assert.equal(await modal().getByPlaceholder("HTTPS 下载 URL").inputValue(), payload.quality.download_url);
                await modal().getByRole("button", { name: "取消", exact: true }).click();
                const priority = page.locator(".g-card").filter({ has: page.locator(".g-name", { hasText: "auto_priority" }) });
                await priority.getByText("优先级故障转移（完整链路）", { exact: true }).waitFor();
                assert.ok((await priority.textContent()).includes("连续正常 2"));
                assert.ok(!(await priority.textContent()).includes("评估中"), "优先级策略不应等待窗口收满");
                await priority.getByText("编辑", { exact: true }).click();
                assert.equal(await modal().getByPlaceholder("HTTPS 下载 URL").inputValue(), "https://priority.example.com/probe.bin");
                await modal().getByText("高优先级连续正常 2 次", { exact: false }).waitFor();
                const choosePolicy = async (label) => {
                    await modal().locator('[aria-label="切换策略"]').click();
                    await page.locator(".ivu-select-dropdown:visible .ivu-select-item").getByText(label, { exact: true }).click();
                };
                await choosePolicy("质量优先（完整链路）");
                await choosePolicy("优先级故障转移（完整链路）");
                assert.equal(await modal().getByPlaceholder("HTTPS 下载 URL").inputValue(), "https://priority.example.com/probe.bin");
                await overflow();
                await screenshot("mobile-priority-config");
                await save().click();
                await modal().waitFor({ state: "hidden" });
                const priorityPayload = writes.filter((w) => w.kind === kind && w.action === "edit").at(-1).payload;
                assert.equal(priorityPayload.policy, "priority");
                assert.equal(priorityPayload.quality.interval_sec, 45);
                assert.equal(priorityPayload.quality.download_url, "https://priority.example.com/probe.bin");
                const legacy = page.locator(".g-card").filter({ has: page.locator(".g-name", { hasText: "auto_legacy" }) });
                await legacy.getByText("优先级故障转移（旧版单跳探测）", { exact: true }).waitFor();
                await legacy.getByText("legacy_tcp：完整链路探测要求 member 的 input 为 socks5x", { exact: true }).waitFor();
                await legacy.getByText("编辑", { exact: true }).click();
                await modal().getByText("保存将升级为完整链路探测", { exact: false }).waitFor();
                await overflow();
                await screenshot("mobile-legacy-config");
                await modal().getByRole("button", { name: "取消", exact: true }).click();
                await page.setViewportSize({ width: 1440, height: 1050 });
                await overflow();
                await readableText();
                await screenshot("desktop-priority-routes");
                await page.setViewportSize({ width: 390, height: 844 });
            }
            await screenshot("mobile-" + kind);
        }
        failList = true;
        await page.getByRole("button", { name: "刷新", exact: true }).click();
        await page.getByText("列表加载失败测试", { exact: true }).waitFor();
        failList = false;
        await page.getByRole("button", { name: "重试", exact: true }).click();
        await page.locator(".g-card").first().waitFor();
        // 所有服务类型均可打开原有配置字段，特别是 SOCKS 的认证与超时。
        for (const type of ["http_proxy", "socks5_server", "socks5x_server"]) {
            const card = page.locator(".g-card").filter({ has: page.locator(".g-name", { hasText: type }) });
            await card.getByText("编辑", { exact: true }).click();
            if (type.includes("socks"))
                assert.ok(await modal().getByText("tcp超时(秒)", { exact: true }).isVisible());
            if (type === "socks5_server")
                assert.ok(await modal().getByText("用户名(可空)", { exact: true }).isVisible());
            await modal().getByRole("button", { name: "取消", exact: true }).click();
        }
        await page.setViewportSize({ width: 320, height: 740 });
        await page.locator(".g-actions .primary").click();
        await overflow();
        await screenshot("small-mobile-form");
        await readableText();
        await page.route("**/api/auth/session", (route) =>
            route.fulfill({ json: { code: 200, data: { authenticated: false } } }),
        );
        await page.goto(base + "/login");
        assert.equal(
            await page
                .locator("label")
                .first()
                .evaluate((el) => getComputedStyle(el).color),
            "rgb(71, 85, 105)",
        );
        await overflow();
        assert.deepEqual(errors, [], "浏览器运行错误");
        console.log(
            "通过：四个页面、9 种宽度、文字对比度与地址字号、长 IPv6、默认运行中筛选、探测历史菜单/自定义时间区间/跨时区/筛选/翻页/错误重试、搜索筛选、保存失败重试、真实字段、四种服务、成员排序、两种入口策略共用探测配置与切换保留、优先级状态、旧版兼容提示、启停失败回退、导入导出、克隆删除、SOCKS5 双阶段检测、MTR 关闭与探测清理。",
        );
    } finally {
        await browser.close();
        await new Promise((resolve) => server.close(resolve));
    }
}
main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
    server.close();
});
