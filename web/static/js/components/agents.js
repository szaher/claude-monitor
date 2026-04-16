/* ==========================================================================
   Claude Monitor — Agents Page
   ========================================================================== */

const AgentsPage = {
    charts: {},
    searchQuery: '',

    async render(container) {
        container.innerHTML = this.template();
        await this.loadData();
    },

    template() {
        return `
            <div class="agents-page">
                <h2>Agents</h2>

                <!-- Stats Cards -->
                <div class="card-grid mt-2" id="agent-stats-cards">
                    <div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Total Agents</div></div>
                    <div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Unique Types</div></div>
                    <div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Avg Duration</div></div>
                </div>

                <!-- Charts Row -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Agent Types Distribution</h3></div>
                        <div class="chart-container" style="max-height:300px;">
                            <canvas id="agent-type-donut"></canvas>
                        </div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Agent Spawn Frequency (by day)</h3></div>
                        <div class="chart-container" style="max-height:300px;">
                            <canvas id="agent-frequency-bar"></canvas>
                        </div>
                    </div>
                </div>

                <!-- Agent Tasks Table -->
                <div class="card mt-2">
                    <div class="card-header">
                        <h3>Agent Tasks</h3>
                        <input type="search" id="agent-search" class="input" placeholder="Search agents..." style="width:220px;" aria-label="Search agents">
                    </div>
                    <div id="agent-tasks-table"></div>
                </div>
            </div>
        `;
    },

    async loadData() {
        this.destroyCharts();

        try {
            const data = await API.get('/api/subagents', { limit: 500 });
            const agents = (data && data.subagents) || [];

            this.renderStatsCards(agents);
            this.renderTypeDonut(agents);
            this.renderFrequencyBar(agents);
            this.renderTasksTable(agents);
            this.bindSearch(agents);
        } catch (err) {
            console.error('Agents page load error:', err);
            App.toast('Failed to load agent data: ' + err.message, 'error');
        }
    },

    /* ---- Stats Cards ---- */
    renderStatsCards(agents) {
        const el = document.getElementById('agent-stats-cards');
        if (!el) return;

        const total = agents.length;
        const uniqueTypes = new Set(agents.map(a => a.agent_type || 'unknown')).size;

        // Average duration
        let durationSum = 0;
        let durationCount = 0;
        agents.forEach(a => {
            if (a.started_at && a.ended_at) {
                const ms = new Date(a.ended_at) - new Date(a.started_at);
                if (ms > 0) {
                    durationSum += ms;
                    durationCount++;
                }
            }
        });
        const avgDurMs = durationCount > 0 ? durationSum / durationCount : 0;
        const avgDurStr = avgDurMs > 0 ? this._formatMs(avgDurMs) : '-';

        el.innerHTML = `
            <div class="card stat-card"><div class="stat-value">${total}</div><div class="stat-label">Total Agents</div></div>
            <div class="card stat-card"><div class="stat-value">${uniqueTypes}</div><div class="stat-label">Unique Types</div></div>
            <div class="card stat-card"><div class="stat-value">${avgDurStr}</div><div class="stat-label">Avg Duration</div></div>
        `;
    },

    /* ---- Agent Types Donut ---- */
    renderTypeDonut(agents) {
        const canvas = document.getElementById('agent-type-donut');
        if (!canvas) return;

        if (agents.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No agent data yet</p></div>';
            return;
        }

        const typeCounts = {};
        agents.forEach(a => {
            const t = a.agent_type || 'unknown';
            typeCounts[t] = (typeCounts[t] || 0) + 1;
        });

        const entries = Object.entries(typeCounts).sort((a, b) => b[1] - a[1]);
        const palette = [
            '#5b6abf', '#2ea87a', '#e5a921', '#d94452', '#3b82c4',
            '#9b59b6', '#e67e22', '#1abc9c', '#e74c3c', '#34495e',
        ];
        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';

        this.charts.typeDonut = new Chart(canvas, {
            type: 'doughnut',
            data: {
                labels: entries.map(e => e[0]),
                datasets: [{
                    data: entries.map(e => e[1]),
                    backgroundColor: entries.map((_, i) => palette[i % palette.length]),
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
                },
            },
        });
    },

    /* ---- Agent Spawn Frequency Bar ---- */
    renderFrequencyBar(agents) {
        const canvas = document.getElementById('agent-frequency-bar');
        if (!canvas) return;

        if (agents.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No agent data yet</p></div>';
            return;
        }

        // Group by date
        const dayCounts = {};
        agents.forEach(a => {
            if (!a.started_at) return;
            const day = a.started_at.slice(0, 10);
            dayCounts[day] = (dayCounts[day] || 0) + 1;
        });

        const sortedDays = Object.keys(dayCounts).sort();
        const last30 = sortedDays.slice(-30);

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.frequencyBar = new Chart(canvas, {
            type: 'bar',
            data: {
                labels: last30.map(d => {
                    const dt = new Date(d + 'T00:00:00');
                    return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
                }),
                datasets: [{
                    label: 'Agents Spawned',
                    data: last30.map(d => dayCounts[d] || 0),
                    backgroundColor: '#9b59b6',
                    borderRadius: 3,
                }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                },
                scales: {
                    x: {
                        ticks: { color: textColor, maxRotation: 45 },
                        grid: { display: false },
                    },
                    y: {
                        beginAtZero: true,
                        ticks: { color: textColor, stepSize: 1 },
                        grid: { color: gridColor, drawBorder: false },
                    },
                },
            },
        });
    },

    /* ---- Agent Tasks Table ---- */
    renderTasksTable(agents, query) {
        const el = document.getElementById('agent-tasks-table');
        if (!el) return;

        let filtered = agents;
        if (query) {
            const q = query.toLowerCase();
            filtered = agents.filter(a =>
                (a.agent_type || '').toLowerCase().includes(q) ||
                (a.description || '').toLowerCase().includes(q) ||
                (a.session_id || '').toLowerCase().includes(q)
            );
        }

        if (filtered.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No agents found</p></div>';
            return;
        }

        // Show newest first
        const sorted = [...filtered].sort((a, b) => {
            if (!a.started_at || !b.started_at) return 0;
            return b.started_at.localeCompare(a.started_at);
        });

        const display = sorted.slice(0, 100);

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr>
                            <th>Type</th>
                            <th>Description</th>
                            <th>Started</th>
                            <th>Duration</th>
                            <th>Session</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${display.map(a => `
                            <tr>
                                <td><span class="badge badge-info">${this._esc(a.agent_type || 'unknown')}</span></td>
                                <td class="truncate" style="max-width:350px;" title="${this._esc(a.description || '')}">${this._esc(a.description || '-')}</td>
                                <td style="white-space:nowrap;">${formatDate(a.started_at)}</td>
                                <td>${formatDuration(a.started_at, a.ended_at)}</td>
                                <td class="truncate font-mono text-sm" style="max-width:120px;" title="${this._esc(a.session_id || '')}">${this._esc((a.session_id || '').slice(0, 8))}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
            ${filtered.length > 100 ? `<div class="text-muted text-sm mt-1">Showing 100 of ${filtered.length} agents</div>` : ''}
        `;
    },

    /* ---- Search Binding ---- */
    bindSearch(agents) {
        const searchInput = document.getElementById('agent-search');
        if (!searchInput) return;

        let debounce;
        searchInput.addEventListener('input', () => {
            clearTimeout(debounce);
            debounce = setTimeout(() => {
                this.searchQuery = searchInput.value;
                this.renderTasksTable(agents, searchInput.value);
            }, 250);
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

    _formatMs(ms) {
        const totalSec = Math.floor(ms / 1000);
        if (totalSec < 60) return totalSec + 's';
        const mins = Math.floor(totalSec / 60);
        const secs = totalSec % 60;
        if (mins < 60) return mins + 'm ' + secs + 's';
        const hrs = Math.floor(mins / 60);
        return hrs + 'h ' + (mins % 60) + 'm';
    },

    destroyCharts() {
        Object.values(this.charts).forEach(c => { try { c.destroy(); } catch (_) {} });
        this.charts = {};
    },

    destroy() {
        this.destroyCharts();
    },
};
