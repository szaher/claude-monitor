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

                <!-- Skills & MCP Row -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Skills Used</h3></div>
                        <div id="skills-table"></div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>MCP Servers</h3></div>
                        <div id="mcp-table"></div>
                    </div>
                </div>
            </div>
        `;
    },

    async loadData() {
        this.destroyCharts();

        try {
            const [toolStats, toolCalls, skillStats, mcpStats, errorData] = await Promise.all([
                API.getToolStats(),
                API.get('/api/tool-calls', { limit: 500 }),
                API.getSkillStats().catch(() => ({ skills: [] })),
                API.getMCPStats().catch(() => ({ servers: [] })),
                API.getErrorStats().catch(() => null),
            ]);

            const tools = (toolStats && toolStats.tools) || [];
            const calls = (toolCalls && toolCalls.tool_calls) || [];
            const skills = (skillStats && skillStats.skills) || [];
            const servers = (mcpStats && mcpStats.servers) || [];

            this.renderUsageDonut(tools);
            this.renderSuccessBar(tools);
            this.renderDurationBar(calls);
            this.renderBashCommands(calls);
            this.renderFilesTable(calls);
            this.renderSkillsTable(skills);
            this.renderMCPTable(servers);
            this.renderErrorAnalysis(errorData);
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

    /* ---- Skills Table ---- */
    renderSkillsTable(skills) {
        const el = document.getElementById('skills-table');
        if (!el) return;

        const sorted = (skills || [])
            .slice()
            .sort((a, b) => (b.count || 0) - (a.count || 0));

        if (sorted.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No skills data found</p></div>';
            return;
        }

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr><th>Skill Name</th><th class="text-right">Count</th></tr>
                    </thead>
                    <tbody>
                        ${sorted.map(s => `
                            <tr>
                                <td><span class="badge badge-info">${this._esc(s.name)}</span></td>
                                <td class="text-right">${s.count || 0}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    },

    /* ---- MCP Servers Table ---- */
    renderMCPTable(servers) {
        const el = document.getElementById('mcp-table');
        if (!el) return;

        const sorted = (servers || [])
            .slice()
            .sort((a, b) => (b.count || 0) - (a.count || 0));

        if (sorted.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No MCP server data found</p></div>';
            return;
        }

        // Flatten server+tool combinations into rows
        const rows = [];
        sorted.forEach(server => {
            const tools = (server.tools || [])
                .slice()
                .sort((a, b) => (b.count || 0) - (a.count || 0));
            if (tools.length === 0) {
                rows.push({ server: server.name, tool: '-', count: server.count || 0 });
            } else {
                tools.forEach(tool => {
                    rows.push({ server: server.name, tool: tool.name, count: tool.count || 0 });
                });
            }
        });

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr><th>Server</th><th>Tool</th><th class="text-right">Count</th></tr>
                    </thead>
                    <tbody>
                        ${rows.map(r => `
                            <tr>
                                <td><span class="badge badge-success">${this._esc(r.server)}</span></td>
                                <td>${this._esc(r.tool)}</td>
                                <td class="text-right">${r.count}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    },

    /* ---- Error Analysis Section ---- */
    renderErrorAnalysis(data) {
        if (!data) return;

        const container = document.querySelector('.tools-page');
        if (!container) return;

        const totalErrors = data.total_errors || 0;
        const errorRate = data.error_rate || 0;
        const errorsByTool = data.errors_by_tool || [];
        const commonErrors = data.common_errors || [];
        const errorTrend = data.error_trend || [];

        // Section header
        const section = document.createElement('div');
        section.className = 'error-analysis-section mt-2';
        section.innerHTML = '<h2 class="mt-2">Error Analysis</h2>';

        // Stat cards
        const statsGrid = document.createElement('div');
        statsGrid.className = 'card-grid mt-2';
        statsGrid.style.gridTemplateColumns = '1fr 1fr';
        statsGrid.innerHTML = `
            <div class="card" style="margin-bottom:0;">
                <div class="card-header"><h3>Total Errors</h3></div>
                <div style="padding:1rem;text-align:center;">
                    <span style="font-size:2.5rem;font-weight:700;color:#d94452;">${totalErrors}</span>
                </div>
            </div>
            <div class="card" style="margin-bottom:0;">
                <div class="card-header"><h3>Error Rate</h3></div>
                <div style="padding:1rem;text-align:center;">
                    <span style="font-size:2.5rem;font-weight:700;color:#e5a921;">${(errorRate * 100).toFixed(1)}%</span>
                </div>
            </div>
        `;
        section.appendChild(statsGrid);

        // Charts row
        const chartsGrid = document.createElement('div');
        chartsGrid.className = 'card-grid mt-2';
        chartsGrid.style.gridTemplateColumns = '1fr 1fr';
        chartsGrid.innerHTML = `
            <div class="card" style="margin-bottom:0;">
                <div class="card-header"><h3>Failure Rate by Tool</h3></div>
                <div class="chart-container" style="max-height:320px;">
                    <canvas id="error-by-tool-chart"></canvas>
                </div>
            </div>
            <div class="card" style="margin-bottom:0;">
                <div class="card-header"><h3>Error Trend (30 Days)</h3></div>
                <div class="chart-container" style="max-height:320px;">
                    <canvas id="error-trend-chart"></canvas>
                </div>
            </div>
        `;
        section.appendChild(chartsGrid);

        // Common error patterns table
        const patternsCard = document.createElement('div');
        patternsCard.className = 'card mt-2';
        if (commonErrors.length === 0) {
            patternsCard.innerHTML = `
                <div class="card-header"><h3>Common Error Patterns</h3></div>
                <div class="empty-state"><p>No error patterns found</p></div>
            `;
        } else {
            patternsCard.innerHTML = `
                <div class="card-header"><h3>Common Error Patterns</h3></div>
                <div class="table-wrapper">
                    <table>
                        <thead>
                            <tr><th>Error Pattern</th><th class="text-right">Count</th></tr>
                        </thead>
                        <tbody>
                            ${commonErrors.map(e => `
                                <tr>
                                    <td class="truncate font-mono text-sm" style="max-width:500px;" title="${this._esc(e.pattern)}">${this._esc(e.pattern)}</td>
                                    <td class="text-right">${e.count}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
            `;
        }
        section.appendChild(patternsCard);

        container.appendChild(section);

        // Render charts
        this.renderErrorByToolChart(errorsByTool);
        this.renderErrorTrendChart(errorTrend);
    },

    renderErrorByToolChart(errorsByTool) {
        const canvas = document.getElementById('error-by-tool-chart');
        if (!canvas) return;

        if (!errorsByTool || errorsByTool.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No error data by tool</p></div>';
            return;
        }

        const sorted = errorsByTool.slice(0, 12);
        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.errorByTool = new Chart(canvas, {
            type: 'bar',
            data: {
                labels: sorted.map(t => t.tool),
                datasets: [{
                    label: 'Failure Rate',
                    data: sorted.map(t => (t.rate * 100).toFixed(1)),
                    backgroundColor: sorted.map(t => t.rate > 0.5 ? '#d94452' : t.rate > 0.2 ? '#e5a921' : '#2ea87a'),
                    borderRadius: 4,
                    barThickness: 20,
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
                                const item = sorted[ctx.dataIndex];
                                return ctx.parsed.x + '% (' + item.errors + '/' + item.total + ')';
                            },
                        },
                    },
                },
                scales: {
                    x: {
                        ticks: { color: textColor, callback: v => v + '%' },
                        grid: { color: gridColor, drawBorder: false },
                        title: { display: true, text: 'Failure Rate (%)', color: textColor },
                        max: 100,
                    },
                    y: {
                        ticks: { color: textColor },
                        grid: { display: false },
                    },
                },
            },
        });
    },

    renderErrorTrendChart(errorTrend) {
        const canvas = document.getElementById('error-trend-chart');
        if (!canvas) return;

        if (!errorTrend || errorTrend.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No error trend data</p></div>';
            return;
        }

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.errorTrend = new Chart(canvas, {
            type: 'line',
            data: {
                labels: errorTrend.map(d => d.date),
                datasets: [
                    {
                        label: 'Errors',
                        data: errorTrend.map(d => d.errors),
                        borderColor: '#d94452',
                        backgroundColor: 'rgba(217, 68, 82, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 3,
                    },
                    {
                        label: 'Total Calls',
                        data: errorTrend.map(d => d.total),
                        borderColor: '#5b6abf',
                        backgroundColor: 'rgba(91, 106, 191, 0.1)',
                        fill: false,
                        tension: 0.3,
                        pointRadius: 3,
                    },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { labels: { color: textColor, usePointStyle: true, padding: 12 } },
                },
                scales: {
                    x: {
                        ticks: { color: textColor, maxTicksLimit: 10 },
                        grid: { display: false },
                    },
                    y: {
                        ticks: { color: textColor },
                        grid: { color: gridColor, drawBorder: false },
                        title: { display: true, text: 'Count', color: textColor },
                    },
                },
            },
        });
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
