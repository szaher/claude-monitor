/* ==========================================================================
   Claude Monitor — Insights Page
   ========================================================================== */

const InsightsPage = {
    charts: {},
    period: 'daily',
    selectedMetric: 'avg_cost_per_session',
    insightsData: null,

    metricConfig: {
        avg_cost_per_session:  { label: 'Avg Cost / Session',  color: '#5b6abf', format: 'cost' },
        cache_hit_rate:        { label: 'Cache Hit Rate',       color: '#2ea87a', format: 'percent' },
        avg_tokens_per_session:{ label: 'Avg Tokens / Session', color: '#e85d5d', format: 'tokens' },
        avg_duration_minutes:  { label: 'Avg Duration',         color: '#f0a030', format: 'duration' },
        error_rate:            { label: 'Error Rate',           color: '#d94452', format: 'percent' },
        sessions_per_day:      { label: 'Sessions / Day',       color: '#5b6abf', format: 'number' },
        tool_calls_per_session:{ label: 'Tool Calls / Session', color: '#7c7c7c', format: 'number' },
        total_cost:            { label: 'Total Cost',           color: '#e85d5d', format: 'cost' },
    },

    async render(container) {
        container.innerHTML = this.template();
        this.bindControls();
        await this.loadData();
    },

    template() {
        return `
            <div class="insights-page">
                <div style="display:flex;justify-content:space-between;align-items:center;">
                    <h2 style="margin:0;">Insights</h2>
                    <a href="#compare" style="font-size:0.85rem;color:var(--text-muted);">&#8644; Compare two sessions</a>
                </div>

                <!-- Controls -->
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem;">
                    <div style="display:flex;gap:8px;align-items:center;">
                        <select id="insights-project" class="input" style="min-width:180px;">
                            <option value="">All Projects</option>
                        </select>
                        <div class="tabs" style="border:none;margin:0;gap:4px;">
                            <button class="tab ${this.period === 'daily' ? 'active' : ''}" data-period="daily">Daily</button>
                            <button class="tab ${this.period === 'weekly' ? 'active' : ''}" data-period="weekly">Weekly</button>
                            <button class="tab ${this.period === 'monthly' ? 'active' : ''}" data-period="monthly">Monthly</button>
                        </div>
                    </div>
                    <span style="font-size:0.8rem;color:var(--text-muted);">Comparing to previous period</span>
                </div>

                <!-- KPI Cards -->
                <div class="card-grid" id="insights-cards" style="grid-template-columns:repeat(4,1fr);">
                    ${Object.keys(this.metricConfig).map(() =>
                        '<div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Loading...</div></div>'
                    ).join('')}
                </div>

                <!-- Expanded Chart -->
                <div class="card mt-2" id="insights-chart-card">
                    <div class="card-header">
                        <h3 id="insights-chart-title">Select a metric</h3>
                    </div>
                    <div class="chart-container" style="max-height:320px;">
                        <canvas id="insights-chart"></canvas>
                    </div>
                </div>
            </div>
        `;
    },

    bindControls() {
        document.querySelectorAll('.insights-page .tab[data-period]').forEach(btn => {
            btn.addEventListener('click', () => {
                this.period = btn.dataset.period;
                const container = document.getElementById('main-content');
                container.innerHTML = '';
                this.render(container);
            });
        });

        this.populateProjects();
        const sel = document.getElementById('insights-project');
        if (sel) {
            sel.addEventListener('change', () => {
                const container = document.getElementById('main-content');
                container.innerHTML = '';
                this.render(container);
            });
        }
    },

    async populateProjects() {
        try {
            const data = await API.getProjectStats();
            const sel = document.getElementById('insights-project');
            if (!sel || !data || !data.projects) return;
            data.projects.forEach(p => {
                const name = p.name || p.path;
                if (!name) return;
                const opt = document.createElement('option');
                opt.value = p.path || name;
                opt.textContent = name;
                sel.appendChild(opt);
            });
        } catch (_) { /* ignore */ }
    },

    async loadData() {
        this.destroyCharts();

        try {
            const params = { period: this.period };
            const sel = document.getElementById('insights-project');
            if (sel && sel.value) params.project = sel.value;

            const filters = App.getFilterParams();
            if (filters.from) params.from = filters.from;
            if (filters.to) params.to = filters.to;

            this.insightsData = await API.getInsightsStats(params);
            this.renderCards();
            this.renderChart();
        } catch (err) {
            console.error('Insights load error:', err);
            App.toast('Failed to load insights: ' + err.message, 'error');
        }
    },

    renderCards() {
        const el = document.getElementById('insights-cards');
        if (!el || !this.insightsData || !this.insightsData.metrics) return;

        const metrics = this.insightsData.metrics;
        el.innerHTML = Object.entries(this.metricConfig).map(([key, cfg]) => {
            const m = metrics[key];
            if (!m) return '';

            const value = this.formatValue(m.current, cfg.format);
            const delta = m.delta_pct;
            const trend = m.trend;
            const isSelected = key === this.selectedMetric;
            const sparkSvg = this.renderSparkline(m.series || [], cfg.color);

            let deltaColor = 'var(--text-muted)';
            if (trend === 'improving') deltaColor = '#2ea87a';
            else if (trend === 'worsening') deltaColor = '#e85d5d';

            const arrow = delta > 0 ? '↑' : delta < 0 ? '↓' : '';
            const borderStyle = isSelected
                ? `border:2px solid ${cfg.color}`
                : 'border:1px solid var(--border-color)';

            return `
                <div class="card stat-card insights-metric-card" data-metric="${key}"
                     style="cursor:pointer;${borderStyle};margin-bottom:0;">
                    <div style="font-size:0.7rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.5px;">${cfg.label}</div>
                    <div style="display:flex;align-items:baseline;gap:8px;margin-top:4px;">
                        <span style="font-size:1.5rem;font-weight:700;">${value}</span>
                        <span style="font-size:0.75rem;color:${deltaColor};">${arrow}${Math.abs(delta).toFixed(1)}%</span>
                    </div>
                    ${sparkSvg}
                </div>
            `;
        }).join('');

        el.querySelectorAll('.insights-metric-card').forEach(card => {
            card.addEventListener('click', () => {
                this.selectedMetric = card.dataset.metric;
                this.renderCards();
                this.renderChart();
            });
        });
    },

    renderSparkline(series, color) {
        if (!series || series.length < 2) return '';

        const values = series.map(p => p.value);
        const min = Math.min(...values);
        const max = Math.max(...values);
        const range = max - min || 1;
        const w = 120;
        const h = 30;

        const points = values.map((v, i) => {
            const x = (i / (values.length - 1)) * w;
            const y = h - ((v - min) / range) * (h - 4) - 2;
            return `${x},${y}`;
        }).join(' ');

        return `<svg width="100%" height="30" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" style="margin-top:6px;">
            <polyline points="${points}" fill="none" stroke="${color}" stroke-width="2"/>
        </svg>`;
    },

    renderChart() {
        this.destroyCharts();
        const canvas = document.getElementById('insights-chart');
        const titleEl = document.getElementById('insights-chart-title');
        if (!canvas || !this.insightsData || !this.insightsData.metrics) return;

        const cfg = this.metricConfig[this.selectedMetric];
        const m = this.insightsData.metrics[this.selectedMetric];
        if (!cfg || !m) return;

        if (titleEl) {
            titleEl.textContent = cfg.label + ' — ' + this.insightsData.period + ', last ' +
                this.insightsData.from + ' to ' + this.insightsData.to;
        }

        const curSeries = m.series || [];
        const prevSeries = m.previous_series || [];

        if (curSeries.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No data for this period</p></div>';
            return;
        }

        const textColor = getComputedStyle(document.documentElement)
            .getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement)
            .getPropertyValue('--border-color').trim() || '#e0e0e8';

        const self = this;

        this.charts.main = new Chart(canvas, {
            type: 'line',
            data: {
                labels: curSeries.map(p => p.date),
                datasets: [
                    {
                        label: 'Current period',
                        data: curSeries.map(p => p.value),
                        borderColor: cfg.color,
                        backgroundColor: cfg.color + '1a',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 3,
                    },
                    {
                        label: 'Previous period',
                        data: prevSeries.map(p => p.value),
                        borderColor: cfg.color + '80',
                        backgroundColor: 'transparent',
                        borderDash: [5, 5],
                        fill: false,
                        tension: 0.3,
                        pointRadius: 2,
                    },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: 'index', intersect: false },
                plugins: {
                    legend: {
                        labels: { color: textColor, usePointStyle: true, padding: 16 },
                    },
                    tooltip: {
                        callbacks: {
                            label: function(ctx) {
                                return ctx.dataset.label + ': ' + self.formatValue(ctx.parsed.y, cfg.format);
                            },
                        },
                    },
                },
                scales: {
                    x: {
                        ticks: { color: textColor, maxRotation: 45 },
                        grid: { display: false },
                    },
                    y: {
                        ticks: {
                            color: textColor,
                            callback: function(v) { return self.formatValue(v, cfg.format); },
                        },
                        grid: { color: gridColor, drawBorder: false },
                    },
                },
            },
        });
    },

    formatValue(v, format) {
        if (v == null || isNaN(v)) return '-';
        switch (format) {
            case 'cost':     return formatCost(v);
            case 'tokens':   return formatTokens(v);
            case 'percent':  return (v * 100).toFixed(1) + '%';
            case 'duration':
                if (v < 1) return '< 1m';
                if (v >= 60) return Math.floor(v / 60) + 'h ' + Math.round(v % 60) + 'm';
                return Math.round(v) + 'm';
            case 'number':   return Number(v).toFixed(1);
            default:         return String(v);
        }
    },

    destroyCharts() {
        Object.values(this.charts).forEach(c => { try { c.destroy(); } catch (_) {} });
        this.charts = {};
    },

    destroy() {
        this.destroyCharts();
    },
};
