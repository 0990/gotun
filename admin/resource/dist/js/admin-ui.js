(function (global) {
    "use strict";

    const paths = {
        tunnel: "M4 7h11m-3-3 3 3-3 3M20 17H9m3-3-3 3 3 3M4 5v4m16 6v4",
        output: "M4 7h7a5 5 0 0 1 5 5v5m-3-3 3 3 3-3M4 17h4",
        input: "M5 4v4a4 4 0 0 0 4 4h10m-3-3 3 3-3 3M5 20v-4a4 4 0 0 1 4-4",
        service: "M5 4h14v6H5zM5 14h14v6H5zM8 7h.01M8 17h.01m4-10h4m-4 10h4",
        search: "M16 16l4 4M17 10.5a6.5 6.5 0 1 1-13 0 6.5 6.5 0 0 1 13 0",
        plus: "M12 5v14M5 12h14",
        import: "M12 3v12m-4-4 4 4 4-4M4 15v5h16v-5",
        refresh: "M20 10A8 8 0 0 0 6 6L3 9m0-6v6h6M4 14a8 8 0 0 0 14 4l3-3m0 6v-6h-6",
        more: "M5 12h.01M12 12h.01M19 12h.01",
        activity: "M3 12h4l3-8 4 16 3-8h4",
        logout: "M9 4H4v16h5m6-12 4 4-4 4M8 12h11",
        down: "m7 10 5 5 5-5",
        info: "M12 11v6m0-10h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
    };
    const components = {
        "g-icon": {
            functional: true,
            props: { name: String },
            render: function (h, ctx) {
                return h("svg", { class: "g-icon", attrs: { viewBox: "0 0 24 24", "aria-hidden": "true" } }, [
                    h("path", { attrs: { d: paths[ctx.props.name] || paths.tunnel } }),
                ]);
            },
        },
        "g-page-heading": {
            props: ["title", "subtitle", "eyebrow", "unit"],
            template:
                '<div class="g-page-heading"><div><div class="g-eyebrow">{{eyebrow}}</div><h1>{{title}}</h1><p>{{subtitle}}</p></div><div class="g-actions"><slot></slot><button class="g-btn primary" @click="$emit(\'create\')"><g-icon name="plus"></g-icon>新建{{unit}}</button></div></div>',
        },
        "g-summary": {
            props: ["rows", "unit", "loading"],
            computed: {
                counts: function () {
                    return countRows(this.rows || []);
                },
            },
            template:
                '<div class="g-summary"><div><strong>{{rows.length}}</strong><span>本页{{unit}}</span></div><div><strong class="g-green">{{counts.running}}</strong><span>运行中</span></div><div><strong>{{counts.off}}</strong><span>已停用</span></div><button class="g-refresh" :disabled="loading" @click="$emit(\'refresh\')"><span class="g-dot"></span>{{loading ? \'正在更新\' : \'刷新列表\'}}<g-icon name="refresh"></g-icon></button></div>',
        },
        "g-list-toolbar": {
            props: ["value", "filter", "counts"],
            template:
                '<div class="g-toolbar"><label class="g-search"><g-icon name="search"></g-icon><input type="search" :value="value" @input="$emit(\'input\', $event.target.value)" placeholder="搜索本页名称、协议或地址…" aria-label="搜索当前页" autocomplete="off"></label><div class="g-toolbar-right"><div class="g-segmented" role="group" aria-label="状态筛选"><button v-for="f in [{id:\'all\',text:\'全部\'},{id:\'running\',text:\'运行中\'},{id:\'off\',text:\'已停用\'}]" :key="f.id" :class="{active: filter === f.id}" :aria-pressed="filter === f.id ? \'true\' : \'false\'" @click="$emit(\'update:filter\',f.id)">{{f.text}} <small>{{counts[f.id]}}</small></button></div><slot></slot></div></div>',
        },
        "g-list-feedback": {
            props: ["loading", "error", "empty"],
            template:
                '<div v-if="error || loading || empty" class="g-feedback" role="status"><template v-if="error"><p>{{error}}</p><button class="g-btn" @click="$emit(\'retry\')">重试</button></template><template v-else-if="loading">正在加载列表…</template><template v-else>暂无匹配记录，可调整筛选或新建配置。</template></div>',
        },
        "g-list-footer": {
            props: ["total", "size", "page", "visible"],
            template:
                '<div class="g-list-footer"><span>本页显示 {{visible}} 条 · 共 {{total}} 条</span><i-page v-if="total > 0" :total="total" :page-size="size || 20" :current="Number(page) || 1" simple @on-change="$emit(\'change\',$event)"></i-page></div>',
        },
    };
    // 子组件局部注册，避免依赖启动包是否暴露全局 Vue。
    Object.keys(components).forEach(function (name) {
        if (name !== "g-icon") components[name].components = { "g-icon": components["g-icon"] };
    });

    function isRunning(row) {
        return !row.disabled && String(row.status).toLowerCase() === "running";
    }
    function countRows(rows) {
        return {
            all: rows.length,
            running: rows.filter(isRunning).length,
            off: rows.filter((r) => r.disabled).length,
        };
    }
    const base = {
        components: components,
        methods: {
            endpointProto: function (value) {
                return global.TunnelFormHelper.splitEndpoint(value, "").proto || "TCP";
            },
            endpointAddr: function (value) {
                return global.TunnelFormHelper.splitEndpoint(value, "").addr || "—";
            },
            isRunning: isRunning,
            stateLabel: function (row) {
                if (row.disabled) return "已停用";
                if (isRunning(row)) return "运行中";
                return row.status === "error" || /run:/.test(row.status || "") ? "运行异常" : "待就绪";
            },
            stateClass: function (row) {
                if (row.disabled) return "off";
                if (isRunning(row)) return "";
                return row.status === "error" || /run:/.test(row.status || "") ? "error" : "waiting";
            },
            healthLabel: function (value) {
                return (
                    { up: "健康", degraded: "不稳定", down: "不可用", disabled: "未探测" }[value] || "未知"
                );
            },
            showError: function (error) {
                this.$Message.error(
                    typeof error === "string"
                        ? error
                        : error && error.message
                          ? error.message
                          : "请求失败，请重试",
                );
            },
        },
    };
    const list = {
        mixins: [base],
        data: function () {
            return {
                searchText: "",
                statusFilter: "all",
                listLoading: true,
                listError: "",
                listRequestId: 0,
                mutationPending: {},
                saving: false,
            };
        },
        computed: {
            memberOptions: function () {
                // 保留已配置但暂未加载或已失效的成员，避免打开编辑框时静默丢失配置。
                const selected = (this.editModalData && this.editModalData.members) || [];
                return Array.from(new Set((this.tunnelOptions || []).concat(selected)));
            },
            listCounts: function () {
                return countRows(this.main_data);
            },
            filteredRows: function () {
                const query = this.searchText.trim().toLowerCase();
                return this.main_data.filter((row) => {
                    if (this.statusFilter === "off" && !row.disabled) return false;
                    if (this.statusFilter === "running" && !isRunning(row)) return false;
                    const text = [row.name, row.input, row.output, row.listen, row.listen_port, row.type]
                        .concat(row.members || [])
                        .join(" ")
                        .toLowerCase();
                    return !query || text.indexOf(query) >= 0;
                });
            },
        },
        watch: {
            "$route.query.page": function (page) {
                this.routePage = page || "";
                this.tableData();
            },
        },
        methods: {
            loadList: function (endpoint, normalize) {
                const requestId = ++this.listRequestId;
                this.listLoading = true;
                this.listError = "";
                const page = this.$route.query.page || "";
                return this.axios
                    .post(endpoint + (page ? "?page=" + encodeURIComponent(page) : ""), this.info || {})
                    .then((response) => {
                        if (requestId !== this.listRequestId) return;
                        if (response.data.code !== 200) throw new Error(response.data.msg || "列表加载失败");
                        const payload = response.data.data;
                        this.main_data = (payload.list || []).map(
                            normalize || ((row) => Object.assign(row, { disabled: !!row.disabled })),
                        );
                        this.pageTotal = payload.pagination.total_nums;
                        this.pageSize = payload.pagination.page_size;
                        if (payload.version)
                            global.dispatchEvent(
                                new CustomEvent("gotun-version", { detail: payload.version }),
                            );
                    })
                    .catch((error) => {
                        if (requestId === this.listRequestId)
                            this.listError = error.message || "列表加载失败，请重试";
                    })
                    .finally(() => {
                        if (requestId === this.listRequestId) this.listLoading = false;
                    });
            },
            saveConfiguration: function (url, payload) {
                if (this.saving) return;
                if (!String(payload.name || "").trim()) {
                    this.showError("请填写名称");
                    return;
                }
                this.saving = true;
                return this.axios
                    .post(url, payload)
                    .then((response) => {
                        if (response.data.code !== 200) throw new Error(response.data.msg || "保存失败");
                        this.$Message.success("已保存");
                        this.editModal = false;
                        return this.tableData();
                    })
                    .catch(this.showError)
                    .finally(() => {
                        this.saving = false;
                    });
            },
            setDisabled: function (row, enabled, endpoint, query) {
                if (this.mutationPending[row.name]) return;
                this.$set(this.mutationPending, row.name, true);
                const disabled = !enabled;
                const suffix = query ? "?name=" + encodeURIComponent(row.name) + "&disabled=" + disabled : "";
                return this.axios
                    .post(endpoint + suffix, query ? {} : { name: row.name, disabled: disabled })
                    .then((response) => {
                        if (response.data.code !== 200) throw new Error(response.data.msg || "更新失败");
                        this.$Message.success(row.name + (disabled ? " 已停用" : " 已启用"));
                        return this.tableData();
                    })
                    .catch(this.showError)
                    .finally(() => {
                        this.$delete(this.mutationPending, row.name);
                    });
            },
        },
        beforeDestroy: function () {
            this.listRequestId++;
        },
    };
    global.GotunUI = { base: base, list: list, components: components };
})(window);
