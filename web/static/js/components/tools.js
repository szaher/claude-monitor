/* ==========================================================================
   Claude Monitor — Tools Page
   ========================================================================== */

const ToolsPage = {
    charts: {},

    async render(container) {
        container.innerHTML = this.template();
        await this.loadData();
    },

    template() {
        return `
            <div class="tools-page">
                <h2>Tools</h2>

                <!-- Charts Row -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Tool Usage Breakdown</h3></div>
                        <div class="chart-container" style="max-height:320px;">
                            <canvas id="tool-usage-donut"></canvas>
                        </div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Success / Failure Rates</h3></div>
                        <div class="chart-container" style="max-height:320px;">
                            <canvas id="tool-success-bar"></canvas>
                        </div>
                    </div>
                </div>

                <!-- Avg Execution Time -->
                <div class="card mt-2">
                    <div class="card-header"><h3>Average Execution Time (ms)</h3></div>
                    <div class="chart-container" style="max-height:300px;">
                        <canvas id="tool-duration-bar"></canvas>
                    </div>
                </div>

                <!-- Tables Row -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Most Common Bash Commands</h3></div>
                        <div id="bash-commands-table"></div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Most Accessed Files</h3></div>
                        <div id="files-table"></div>
                    </div>
                </div>
            </div>
        `;
    },

    async loadData() {
        this.destroyCharts();

        try {
            const [toolStats, toolCalls] = await Promise.all([
                API.getToolStats(),
                API.get('/api/tool-calls', { limit: 500 }),
            ]);

            const tools = (toolStats && toolStats.tools) || [];
            const calls = (toolCalls && toolCalls.tool_calls) || [];

            this.renderUsageDonut(tools);
            this.renderSuccessBar(tools);
            this.renderDurationBar(calls);
            this.renderBashCommands(calls);
            this.renderFilesTable(calls);
        } catch (err) {
            console.error('Tools page load error:', err);
            App.toast('Failed to load tool data: ' + err.message, 'error');
        }
    },

    /* ---- Tool Usage Donut ---- */
    renderUsageDonut(tools) {
        const canvas = document.getElementById('tool-usage-donut');
        if (!canvas) return;

        if (!tools || tools.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No tool data yet</p></div>';
            return;
        }

        const palette = [
            '#5b6abf', '#2ea87a', '#e5a921', '#d94452', '#3b82c4',
            '#9b59b6', '#e67e22', '#1abc9c', '#e74c3c', '#34495e',
            '#16a085', '#f39c12', '#8e44ad', '#2980b9', '#c0392b',
        ];
        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';

        this.charts.usageDonut = new Chart(canvas, {
            type: 'doughnut',
            data: {
                labels: tools.map(t => t.name || 'Unknown'),
                datasets: [{
                    data: tools.map(t => t.count || 0),
                    backgroundColor: tools.map((_, i) => palette[i % palette.length]),
                    borderWidth: 0,
                }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        position: 'right',
                        labels: { color: textColor, usePointStyle: true, padding: 12, font: { size: 12 } },
                    },
                    tooltip: {
                        callbacks: {
                            label: function(ctx) {
                                const total = ctx.dataset.data.reduce((a, b) => a + b, 0);
                                const pct = total > 0 ? ((ctx.parsed / total) * 100).toFixed(1) : 0;
                                return ctx.label + ': ' + ctx.parsed + ' (' + pct + '%)';
                            },
                        },
                    },
                },
            },
        });
    },

    /* ---- Success/Failure Stacked Bar ---- */
    renderSuccessBar(tools) {
        const canvas = document.getElementById('tool-success-bar');
        if (!canvas) return;

        if (!tools || tools.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No tool data yet</p></div>';
            return;
        }

        const top10 = tools.slice(0, 10);
        const labels = top10.map(t => t.name || 'Unknown');
        const successCounts = top10.map(t => Math.round((t.success_rate || 0) * (t.count || 0)));
        const failureCounts = top10.map((t, i) => (t.count || 0) - successCounts[i]);

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.successBar = new Chart(canvas, {
            type: 'bar',
            data: {
                labels,
                datasets: [
                    {
                        label: 'Success',
                        data: successCounts,
                        backgroundColor: '#2ea87a',
                        borderRadius: 2,
                    },
                    {
                        label: 'Failure',
                        data: failureCounts,
                        backgroundColor: '#d94452',
                        borderRadius: 2,
                    },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { labels: { color: textColor, usePointStyle: true, padding: 12 } },
                    tooltip: {
                        callbacks: {
                            afterBody: function(items) {
                                if (items.length === 0) return '';
                                const idx = items[0].dataIndex;
                                const tool = top10[idx];
                                if (tool && tool.success_rate != null) {
                                    return 'Success rate: ' + (tool.success_rate * 100).toFixed(1) + '%';
                                }
                                return '';
                            },
                        },
                    },
                },
                scales: {
                    x: {
                        stacked: true,
                        ticks: { color: textColor },
                        grid: { display: false },
                    },
                    y: {
                        stacked: true,
                        ticks: { color: textColor },
                        grid: { color: gridColor, drawBorder: false },
                        title: { display: true, text: 'Count', color: textColor },
                    },
                },
            },
        });
    },

    /* ---- Average Execution Time Bar ---- */
    renderDurationBar(calls) {
        const canvas = document.getElementById('tool-duration-bar');
        if (!canvas) return;

        // Compute average duration per tool
        const durationMap = {};
        calls.forEach(tc => {
            const name = tc.tool_name || 'Unknown';
            const dur = tc.duration_ms || 0;
            if (dur <= 0) return;
            if (!durationMap[name]) durationMap[name] = { total: 0, count: 0 };
            durationMap[name].total += dur;
            durationMap[name].count++;
        });

        const entries = Object.entries(durationMap)
            .map(([name, d]) => ({ name, avg: Math.round(d.total / d.count) }))
            .sort((a, b) => b.avg - a.avg)
            .slice(0, 12);

        if (entries.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No execution time data available</p></div>';
            return;
        }

        const palette = [
            '#5b6abf', '#2ea87a', '#e5a921', '#d94452', '#3b82c4',
            '#9b59b6', '#e67e22', '#1abc9c', '#e74c3c', '#34495e',
            '#16a085', '#f39c12',
        ];
        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.durationBar = new Chart(canvas, {
            type: 'bar',
            data: {
                labels: entries.map(e => e.name),
                datasets: [{
                    label: 'Avg Duration (ms)',
                    data: entries.map(e => e.avg),
                    backgroundColor: entries.map((_, i) => palette[i % palette.length]),
                    borderRadius: 4,
                    barThickness: 24,
                }],
            },
            options: {
                indexAxis: 'y',
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: function(ctx) {
                                return ctx.parsed.x + ' ms';
                            },
                        },
                    },
                },
                scales: {
                    x: {
                        ticks: { color: textColor },
                        grid: { color: gridColor, drawBorder: false },
                        title: { display: true, text: 'Milliseconds', color: textColor },
                    },
                    y: {
                        ticks: { color: textColor },
                        grid: { display: false },
                    },
                },
            },
        });
    },

    /* ---- Most Common Bash Commands Table ---- */
    renderBashCommands(calls) {
        const el = document.getElementById('bash-commands-table');
        if (!el) return;

        const commandCounts = {};
        calls.forEach(tc => {
            if ((tc.tool_name || '').toLowerCase() !== 'bash') return;
            let cmd = '';
            try {
                const input = typeof tc.tool_input === 'string' ? JSON.parse(tc.tool_input) : tc.tool_input;
                cmd = (input && input.command) || '';
            } catch (_) {
                cmd = tc.tool_input || '';
            }
            if (!cmd) return;
            // Extract just the base command (first word/token)
            const base = cmd.trim().split(/\s+/)[0].replace(/^[./]+/, '');
            if (!base) return;
            commandCounts[base] = (commandCounts[base] || 0) + 1;
        });

        const sorted = Object.entries(commandCounts)
            .map(([cmd, count]) => ({ cmd, count }))
            .sort((a, b) => b.count - a.count)
            .slice(0, 15);

        if (sorted.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No Bash commands found</p></div>';
            return;
        }

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr><th>Command</th><th class="text-right">Count</th></tr>
                    </thead>
                    <tbody>
                        ${sorted.map(s => `
                            <tr>
                                <td><code>${this._esc(s.cmd)}</code></td>
                                <td class="text-right">${s.count}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    },

    /* ---- Most Accessed Files Table ---- */
    renderFilesTable(calls) {
        const el = document.getElementById('files-table');
        if (!el) return;

        const fileCounts = {};
        const fileTools = ['Read', 'Write', 'Edit', 'read', 'write', 'edit'];
        calls.forEach(tc => {
            const toolName = tc.tool_name || '';
            if (!fileTools.includes(toolName)) return;
            let filePath = '';
            try {
                const input = typeof tc.tool_input === 'string' ? JSON.parse(tc.tool_input) : tc.tool_input;
                filePath = (input && input.file_path) || '';
            } catch (_) {
                filePath = '';
            }
            if (!filePath) return;
            fileCounts[filePath] = (fileCounts[filePath] || 0) + 1;
        });

        const sorted = Object.entries(fileCounts)
            .map(([path, count]) => ({ path, count }))
            .sort((a, b) => b.count - a.count)
            .slice(0, 15);

        if (sorted.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No file access data found</p></div>';
            return;
        }

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr><th>File Path</th><th class="text-right">Accesses</th></tr>
                    </thead>
                    <tbody>
                        ${sorted.map(s => `
                            <tr>
                                <td class="truncate font-mono text-sm" style="max-width:350px;" title="${this._esc(s.path)}">${this._esc(s.path)}</td>
                                <td class="text-right">${s.count}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    },

    /* ---- Helpers ---- */
    _esc(str) {
        if (str == null) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    },

    destroyCharts() {
        Object.values(this.charts).forEach(c => { try { c.destroy(); } catch (_) {} });
        this.charts = {};
    },

    destroy() {
        this.destroyCharts();
    },
};
