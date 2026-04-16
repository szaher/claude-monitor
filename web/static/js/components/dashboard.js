/* ==========================================================================
   Claude Monitor — Dashboard Page
   ========================================================================== */

/* ---- Utility helpers ---- */

/**
 * Format a large number into a compact string (e.g. 1200 -> "1.2K").
 */
function formatTokens(n) {
    if (n == null || isNaN(n)) return '0';
    n = Number(n);
    if (n >= 1_000_000_000) return (n / 1_000_000_000).toFixed(1).replace(/\.0$/, '') + 'B';
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'K';
    return n.toLocaleString();
}

/**
 * Format a number as USD currency (e.g. 3.42 -> "$3.42").
 */
function formatCost(n) {
    if (n == null || isNaN(n)) return '$0.00';
    return '$' + Number(n).toFixed(2);
}

/**
 * Format a duration between two ISO timestamps (e.g. "2h 15m", "45m", "< 1m").
 * If endedAt is falsy the session is still running.
 */
function formatDuration(startedAt, endedAt) {
    if (!startedAt) return '-';
    if (!endedAt) return 'ongoing';
    const ms = new Date(endedAt) - new Date(startedAt);
    if (ms < 0) return '-';
    const totalMin = Math.floor(ms / 60000);
    if (totalMin < 1) return '< 1m';
    const hours = Math.floor(totalMin / 60);
    const mins = totalMin % 60;
    if (hours > 0) return hours + 'h ' + mins + 'm';
    return mins + 'm';
}

/**
 * Format an ISO date string to a readable short format (e.g. "Apr 16, 2:30 PM").
 */
function formatDate(iso) {
    if (!iso) return '-';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '-';
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) +
        ', ' +
        d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
}

/* ---- Dashboard Page Component ---- */

const DashboardPage = {
    charts: {},

    async render(container) {
        container.innerHTML = this.template();
        await this.loadData();
    },

    template() {
        return `
            <div class="dashboard">
                <h2>Dashboard</h2>

                <!-- Stats Cards -->
                <div class="card-grid mt-2" id="stats-cards">
                    ${this._statCardPlaceholder('Sessions Today', '--')}
                    ${this._statCardPlaceholder('Tool Calls Today', '--')}
                    ${this._statCardPlaceholder('Tokens Today', '--')}
                    ${this._statCardPlaceholder('Cost Today', '--')}
                </div>

                <!-- Activity Heatmap -->
                <div class="card mt-2">
                    <div class="card-header"><h3>Activity</h3></div>
                    <div id="heatmap-container" style="overflow-x:auto;"></div>
                </div>

                <!-- Charts row -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Token Usage (30 days)</h3></div>
                        <div class="chart-container"><canvas id="token-chart"></canvas></div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Top Tools</h3></div>
                        <div class="chart-container"><canvas id="tools-chart"></canvas></div>
                    </div>
                </div>

                <!-- Projects + Recent Sessions row -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 2fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Top Projects</h3></div>
                        <div id="projects-list"></div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Recent Sessions</h3></div>
                        <div id="recent-sessions"></div>
                    </div>
                </div>
            </div>
        `;
    },

    _statCardPlaceholder(label, value) {
        return `<div class="card stat-card"><div class="stat-value">${value}</div><div class="stat-label">${label}</div></div>`;
    },

    /* ------------------------------------------------------------------
       Data Loading
    ------------------------------------------------------------------ */
    async loadData() {
        // Destroy any previous Chart.js instances to avoid canvas reuse errors
        Object.values(this.charts).forEach(c => { try { c.destroy(); } catch (_) { /* noop */ } });
        this.charts = {};

        try {
            const [stats, daily, tools, projects, sessions, patterns] = await Promise.all([
                API.getStats(),
                API.getDailyStats(365),
                API.getToolStats(),
                API.getProjectStats(),
                API.getSessions({ limit: 10 }),
                API.getPromptPatterns().catch(() => null),
            ]);

            this.renderStatCards(stats);
            this.renderHeatmap((daily && daily.days) || []);
            this.renderTokenChart((daily && daily.days) || []);
            this.renderToolsChart((tools && tools.tools) || []);
            this.renderProjectsList((projects && projects.projects) || []);
            this.renderRecentSessions((sessions && sessions.sessions) || []);
            this.renderPatterns(patterns);
        } catch (err) {
            console.error('Dashboard load error:', err);
            if (typeof App !== 'undefined' && App.toast) {
                App.toast('Failed to load dashboard data: ' + err.message, 'error');
            }
        }
    },

    /* ------------------------------------------------------------------
       1. Stat Cards
    ------------------------------------------------------------------ */
    renderStatCards(stats) {
        const el = document.getElementById('stats-cards');
        if (!el) return;

        const today = (stats && stats.today) || {};
        el.innerHTML = [
            this._statCardPlaceholder('Sessions Today', today.sessions != null ? today.sessions : 0),
            this._statCardPlaceholder('Tool Calls Today', today.tool_calls != null ? today.tool_calls : 0),
            this._statCardPlaceholder('Tokens Today', formatTokens(today.tokens || 0)),
            this._statCardPlaceholder('Cost Today', formatCost(today.cost || 0)),
        ].join('');
    },

    /* ------------------------------------------------------------------
       2. Activity Heatmap (GitHub-style)
    ------------------------------------------------------------------ */
    renderHeatmap(days) {
        const container = document.getElementById('heatmap-container');
        if (!container) return;

        if (!days || days.length === 0) {
            container.innerHTML = '<div class="empty-state"><p>No activity data yet</p></div>';
            return;
        }

        // Build a map of date -> sessions for fast lookup
        const dateMap = {};
        let maxSessions = 0;
        days.forEach(d => {
            dateMap[d.date] = d.sessions || 0;
            if ((d.sessions || 0) > maxSessions) maxSessions = d.sessions;
        });

        // Generate last 52 weeks (364 days) of dates ending today
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        const totalDays = 364;
        const startDate = new Date(today);
        startDate.setDate(startDate.getDate() - totalDays);
        // Align to start of week (Sunday)
        const startDayOfWeek = startDate.getDay(); // 0=Sun
        startDate.setDate(startDate.getDate() - startDayOfWeek);

        const weeks = [];
        const cellSize = 13;
        const cellGap = 3;
        const dayLabels = ['', 'Mon', '', 'Wed', '', 'Fri', ''];

        const cursor = new Date(startDate);
        while (cursor <= today) {
            const week = [];
            for (let dow = 0; dow < 7; dow++) {
                const dateStr = cursor.toISOString().slice(0, 10);
                const count = dateMap[dateStr] || 0;
                const isAfterToday = cursor > today;
                week.push({ date: dateStr, count, inactive: isAfterToday });
                cursor.setDate(cursor.getDate() + 1);
            }
            weeks.push(week);
        }

        // Color scale (GitHub green style)
        const getColor = (count) => {
            if (count === 0) return 'var(--bg-tertiary)';
            if (maxSessions <= 0) return 'var(--bg-tertiary)';
            const ratio = count / maxSessions;
            if (ratio <= 0.25) return '#0e4429';
            if (ratio <= 0.50) return '#006d32';
            if (ratio <= 0.75) return '#26a641';
            return '#39d353';
        };

        // Build month labels
        const monthNames = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
        let monthLabelsHTML = '';
        let lastMonth = -1;
        weeks.forEach((week, wi) => {
            const firstDayDate = new Date(week[0].date);
            const m = firstDayDate.getMonth();
            if (m !== lastMonth) {
                lastMonth = m;
                const x = wi * (cellSize + cellGap);
                monthLabelsHTML += `<text x="${x}" y="10" class="heatmap-month-label">${monthNames[m]}</text>`;
            }
        });

        // Build SVG
        const svgWidth = weeks.length * (cellSize + cellGap) + 30;
        const svgHeight = 7 * (cellSize + cellGap) + 24;
        const offsetY = 18;
        const offsetX = 28;

        let cellsHTML = '';
        weeks.forEach((week, wi) => {
            week.forEach((day, di) => {
                if (day.inactive) return;
                const x = offsetX + wi * (cellSize + cellGap);
                const y = offsetY + di * (cellSize + cellGap);
                const color = getColor(day.count);
                cellsHTML += `<rect x="${x}" y="${y}" width="${cellSize}" height="${cellSize}" rx="2" ry="2" fill="${color}" data-date="${day.date}" data-count="${day.count}"><title>${day.date}: ${day.count} session${day.count !== 1 ? 's' : ''}</title></rect>`;
            });
        });

        // Day labels
        let dayLabelsHTML = '';
        dayLabels.forEach((lbl, i) => {
            if (lbl) {
                const y = offsetY + i * (cellSize + cellGap) + cellSize - 2;
                dayLabelsHTML += `<text x="0" y="${y}" class="heatmap-day-label">${lbl}</text>`;
            }
        });

        // Shift month labels right by offset
        const adjustedMonthLabels = monthLabelsHTML.replace(/x="(\d+)"/g, (_, n) => `x="${Number(n) + offsetX}"`);

        container.innerHTML = `
            <style>
                .heatmap-month-label, .heatmap-day-label {
                    fill: var(--text-muted);
                    font-size: 10px;
                    font-family: var(--font-sans);
                }
                .heatmap-day-label {
                    font-size: 9px;
                }
            </style>
            <svg width="${svgWidth + offsetX}" height="${svgHeight}" style="display:block;">
                ${adjustedMonthLabels}
                ${dayLabelsHTML}
                ${cellsHTML}
            </svg>
        `;
    },

    /* ------------------------------------------------------------------
       3. Token Usage Over Time (Chart.js line chart, last 30 days)
    ------------------------------------------------------------------ */
    renderTokenChart(days) {
        const canvas = document.getElementById('token-chart');
        if (!canvas) return;

        // Use last 30 days of data
        const last30 = (days || []).slice(-30);

        if (last30.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No token data yet</p></div>';
            return;
        }

        const labels = last30.map(d => {
            const dt = new Date(d.date + 'T00:00:00');
            return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
        });
        const tokenData = last30.map(d => d.tokens || 0);
        const costData = last30.map(d => d.cost || 0);

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.tokenChart = new Chart(canvas, {
            type: 'line',
            data: {
                labels,
                datasets: [
                    {
                        label: 'Tokens',
                        data: tokenData,
                        borderColor: '#5b6abf',
                        backgroundColor: 'rgba(91, 106, 191, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 2,
                        pointHoverRadius: 5,
                        yAxisID: 'y',
                    },
                    {
                        label: 'Cost ($)',
                        data: costData,
                        borderColor: '#2ea87a',
                        backgroundColor: 'rgba(46, 168, 122, 0.1)',
                        fill: false,
                        tension: 0.3,
                        pointRadius: 2,
                        pointHoverRadius: 5,
                        yAxisID: 'y1',
                    },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: 'index', intersect: false },
                plugins: {
                    legend: { labels: { color: textColor, usePointStyle: true, padding: 16 } },
                    tooltip: {
                        callbacks: {
                            label: function(ctx) {
                                if (ctx.dataset.label === 'Cost ($)') return 'Cost: ' + formatCost(ctx.parsed.y);
                                return 'Tokens: ' + formatTokens(ctx.parsed.y);
                            },
                        },
                    },
                },
                scales: {
                    x: {
                        ticks: { color: textColor, maxRotation: 45 },
                        grid: { color: gridColor, drawBorder: false },
                    },
                    y: {
                        type: 'linear',
                        position: 'left',
                        title: { display: true, text: 'Tokens', color: textColor },
                        ticks: {
                            color: textColor,
                            callback: function(v) { return formatTokens(v); },
                        },
                        grid: { color: gridColor, drawBorder: false },
                    },
                    y1: {
                        type: 'linear',
                        position: 'right',
                        title: { display: true, text: 'Cost ($)', color: textColor },
                        ticks: {
                            color: textColor,
                            callback: function(v) { return formatCost(v); },
                        },
                        grid: { drawOnChartArea: false },
                    },
                },
            },
        });
    },

    /* ------------------------------------------------------------------
       4. Most Used Tools (horizontal bar chart)
    ------------------------------------------------------------------ */
    renderToolsChart(tools) {
        const canvas = document.getElementById('tools-chart');
        if (!canvas) return;

        if (!tools || tools.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No tool data yet</p></div>';
            return;
        }

        const top10 = tools.slice(0, 10);
        const labels = top10.map(t => t.name || 'Unknown');
        const counts = top10.map(t => t.count || 0);

        const palette = [
            '#5b6abf', '#2ea87a', '#e5a921', '#d94452', '#3b82c4',
            '#9b59b6', '#e67e22', '#1abc9c', '#e74c3c', '#34495e',
        ];
        const bgColors = top10.map((_, i) => palette[i % palette.length]);

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.toolsChart = new Chart(canvas, {
            type: 'bar',
            data: {
                labels,
                datasets: [{
                    label: 'Uses',
                    data: counts,
                    backgroundColor: bgColors,
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
                            afterLabel: function(ctx) {
                                const tool = top10[ctx.dataIndex];
                                if (tool && tool.success_rate != null) {
                                    return 'Success rate: ' + (tool.success_rate * 100).toFixed(0) + '%';
                                }
                                return '';
                            },
                        },
                    },
                },
                scales: {
                    x: {
                        ticks: { color: textColor },
                        grid: { color: gridColor, drawBorder: false },
                        title: { display: true, text: 'Count', color: textColor },
                    },
                    y: {
                        ticks: { color: textColor },
                        grid: { display: false },
                    },
                },
            },
        });
    },

    /* ------------------------------------------------------------------
       5. Top Projects
    ------------------------------------------------------------------ */
    renderProjectsList(projects) {
        const el = document.getElementById('projects-list');
        if (!el) return;

        if (!projects || projects.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No projects yet</p></div>';
            return;
        }

        const top5 = projects.slice(0, 5);
        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr>
                            <th>Project</th>
                            <th class="text-right">Sessions</th>
                            <th class="text-right">Cost</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${top5.map(p => `
                            <tr class="cursor-pointer" onclick="window.location.hash='projects'">
                                <td>
                                    <div class="font-bold">${this._esc(p.name || p.path || 'Unknown')}</div>
                                    <div class="text-muted text-sm truncate" style="max-width:200px;" title="${this._esc(p.path || '')}">${this._esc(p.path || '')}</div>
                                </td>
                                <td class="text-right">${p.sessions || 0}</td>
                                <td class="text-right">${formatCost(p.cost)}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    },

    /* ------------------------------------------------------------------
       6. Recent Sessions
    ------------------------------------------------------------------ */
    renderRecentSessions(sessions) {
        const el = document.getElementById('recent-sessions');
        if (!el) return;

        if (!sessions || sessions.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No sessions yet</p></div>';
            return;
        }

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr>
                            <th>Date/Time</th>
                            <th>Project</th>
                            <th>Duration</th>
                            <th>Tokens</th>
                            <th class="text-right">Cost</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${sessions.map(s => {
                            const totalTokens = (s.total_input_tokens || 0) + (s.total_output_tokens || 0);
                            return `
                                <tr class="cursor-pointer" onclick="window.location.hash='sessions'">
                                    <td style="white-space:nowrap;">${formatDate(s.started_at)}</td>
                                    <td class="truncate" style="max-width:160px;" title="${this._esc(s.project_name || s.project_path || '')}">${this._esc(s.project_name || '-')}</td>
                                    <td>${formatDuration(s.started_at, s.ended_at)}</td>
                                    <td>${formatTokens(totalTokens)}</td>
                                    <td class="text-right">${formatCost(s.estimated_cost_usd)}</td>
                                </tr>
                            `;
                        }).join('')}
                    </tbody>
                </table>
            </div>
        `;
    },

    /* ------------------------------------------------------------------
       7. Usage Patterns (prompt category analysis)
    ------------------------------------------------------------------ */
    renderPatterns(patterns) {
        if (!patterns || !patterns.categories || patterns.categories.length === 0) return;

        const container = document.querySelector('.dashboard');
        if (!container) return;

        const sorted = patterns.categories.sort((a, b) => b.count - a.count);

        const section = document.createElement('div');
        section.className = 'mt-2';
        section.innerHTML = `
            <h3 style="margin-bottom:1rem">Usage Patterns</h3>
            <div class="card-grid" style="grid-template-columns: 1fr 1fr;">
                <div class="card" style="margin-bottom:0;">
                    <div class="card-header"><h3>Prompt Categories</h3></div>
                    <div class="chart-container" style="max-height:280px;">
                        <canvas id="pattern-donut"></canvas>
                    </div>
                </div>
                <div class="card" style="margin-bottom:0;">
                    <div class="card-header"><h3>Category Breakdown</h3></div>
                    <div class="table-wrapper">
                        <table>
                            <thead><tr><th>Category</th><th class="text-right">Count</th><th class="text-right">%</th></tr></thead>
                            <tbody>${sorted.map(c =>
                                `<tr><td>${c.name}</td><td class="text-right">${c.count}</td><td class="text-right">${c.percentage.toFixed(1)}%</td></tr>`
                            ).join('')}</tbody>
                        </table>
                    </div>
                </div>
            </div>
        `;
        container.appendChild(section);

        const colors = ['#3b82f6','#22c55e','#f59e0b','#ef4444','#a855f7','#06b6d4','#ec4899','#84cc16'];
        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';

        this.charts.patternDonut = new Chart(document.getElementById('pattern-donut'), {
            type: 'doughnut',
            data: {
                labels: sorted.map(c => c.name),
                datasets: [{
                    data: sorted.map(c => c.count),
                    backgroundColor: colors.slice(0, sorted.length),
                    borderWidth: 0,
                }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { position: 'bottom', labels: { color: textColor, usePointStyle: true, padding: 8, font: { size: 11 } } },
                },
            },
        });
    },

    /* ------------------------------------------------------------------
       Helpers
    ------------------------------------------------------------------ */

    /** Minimal HTML escaping to avoid XSS in dynamic content. */
    _esc(str) {
        if (str == null) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    },
};
