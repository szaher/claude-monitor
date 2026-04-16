/* ==========================================================================
   Claude Monitor — Cost Page
   ========================================================================== */

const CostPage = {
    charts: {},
    period: 'daily', // 'daily' | 'weekly' | 'monthly'

    async render(container) {
        container.innerHTML = this.template();
        this.bindPeriodToggle();
        await this.loadData();
    },

    template() {
        return `
            <div class="cost-page">
                <h2>Cost Analysis</h2>

                <!-- Summary Cards -->
                <div class="card-grid mt-2" id="cost-summary-cards">
                    <div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Total Spend</div></div>
                    <div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Avg Daily Cost</div></div>
                    <div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Most Expensive Project</div></div>
                    <div class="card stat-card"><div class="stat-value">--</div><div class="stat-label">Cache Savings</div></div>
                </div>

                <!-- Cost Timeline -->
                <div class="card mt-2">
                    <div class="card-header">
                        <h3>Cost Over Time</h3>
                        <div class="tabs" style="border:none;margin:0;gap:4px;">
                            <button class="tab ${this.period === 'daily' ? 'active' : ''}" data-period="daily">Daily</button>
                            <button class="tab ${this.period === 'weekly' ? 'active' : ''}" data-period="weekly">Weekly</button>
                            <button class="tab ${this.period === 'monthly' ? 'active' : ''}" data-period="monthly">Monthly</button>
                        </div>
                    </div>
                    <div class="chart-container" style="max-height:320px;">
                        <canvas id="cost-timeline-chart"></canvas>
                    </div>
                </div>

                <!-- Charts Row -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Cost by Model</h3></div>
                        <div class="chart-container" style="max-height:280px;">
                            <canvas id="cost-model-chart"></canvas>
                        </div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Cost by Project</h3></div>
                        <div class="chart-container" style="max-height:280px;">
                            <canvas id="cost-project-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- Cache Analysis -->
                <div class="card mt-2">
                    <div class="card-header"><h3>Cache Performance</h3></div>
                    <div id="cache-analysis"></div>
                </div>

                <!-- Cost Projection -->
                <div class="card mt-2">
                    <div class="card-header"><h3>Cost Projection (next 30 days)</h3></div>
                    <div id="cost-projection"></div>
                </div>
            </div>
        `;
    },

    bindPeriodToggle() {
        document.querySelectorAll('.cost-page .tab[data-period]').forEach(btn => {
            btn.addEventListener('click', () => {
                this.period = btn.dataset.period;
                const container = document.getElementById('main-content');
                container.innerHTML = '';
                this.render(container);
            });
        });
    },

    async loadData() {
        this.destroyCharts();

        try {
            const [dailyData, modelData, projectData, efficiencyData, budgetStatus] = await Promise.all([
                API.getDailyStats(90),
                API.getModelStats(),
                API.getProjectStats(),
                API.getTokenEfficiency().catch(() => null),
                API.getBudgetStatus().catch(() => null),
            ]);

            const days = (dailyData && dailyData.days) || [];
            const models = (modelData && modelData.models) || [];
            const projects = (projectData && projectData.projects) || [];

            this.renderSummaryCards(days, projects);
            this.renderTimeline(days);
            this.renderModelChart(models);
            this.renderProjectChart(projects);
            this.renderCacheAnalysis(days);
            this.renderCostProjection(days);
            this.renderTokenEfficiency(efficiencyData);
            this.renderBudgets(budgetStatus);
        } catch (err) {
            console.error('Cost page load error:', err);
            App.toast('Failed to load cost data: ' + err.message, 'error');
        }
    },

    /* ---- Summary Cards ---- */
    renderSummaryCards(days, projects) {
        const el = document.getElementById('cost-summary-cards');
        if (!el) return;

        const totalSpend = days.reduce((sum, d) => sum + (d.cost || 0), 0);
        const avgDaily = days.length > 0 ? totalSpend / days.length : 0;

        // Most expensive project
        let expProject = '-';
        if (projects.length > 0) {
            const sorted = [...projects].sort((a, b) => (b.cost || 0) - (a.cost || 0));
            expProject = sorted[0].name || sorted[0].path || 'Unknown';
        }

        // Cache savings estimate (rough: assume cache_read is 10x cheaper than normal input)
        // We don't have cache tokens from daily stats, so just show total spend info
        const cacheSavingsLabel = 'See below';

        el.innerHTML = `
            <div class="card stat-card"><div class="stat-value">${formatCost(totalSpend)}</div><div class="stat-label">Total Spend (${days.length}d)</div></div>
            <div class="card stat-card"><div class="stat-value">${formatCost(avgDaily)}</div><div class="stat-label">Avg Daily Cost</div></div>
            <div class="card stat-card"><div class="stat-value" style="font-size:1.2rem;">${this._esc(expProject)}</div><div class="stat-label">Most Expensive Project</div></div>
            <div class="card stat-card"><div class="stat-value" style="font-size:1rem;">${cacheSavingsLabel}</div><div class="stat-label">Cache Savings</div></div>
        `;
    },

    /* ---- Cost Timeline ---- */
    renderTimeline(days) {
        const canvas = document.getElementById('cost-timeline-chart');
        if (!canvas) return;

        if (!days || days.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No cost data yet</p></div>';
            return;
        }

        let labels, costData;

        if (this.period === 'weekly') {
            // Aggregate into weeks
            const weeks = {};
            days.forEach(d => {
                const dt = new Date(d.date + 'T00:00:00');
                // Get Monday of that week
                const day = dt.getDay();
                const diff = dt.getDate() - day + (day === 0 ? -6 : 1);
                const monday = new Date(dt.setDate(diff));
                const key = monday.toISOString().slice(0, 10);
                weeks[key] = (weeks[key] || 0) + (d.cost || 0);
            });
            const sortedWeeks = Object.keys(weeks).sort();
            labels = sortedWeeks.map(w => {
                const dt = new Date(w + 'T00:00:00');
                return 'W/O ' + dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
            });
            costData = sortedWeeks.map(w => weeks[w]);
        } else if (this.period === 'monthly') {
            // Aggregate into months
            const months = {};
            days.forEach(d => {
                const key = d.date.slice(0, 7); // YYYY-MM
                months[key] = (months[key] || 0) + (d.cost || 0);
            });
            const sortedMonths = Object.keys(months).sort();
            labels = sortedMonths.map(m => {
                const [y, mo] = m.split('-');
                const dt = new Date(parseInt(y), parseInt(mo) - 1);
                return dt.toLocaleDateString('en-US', { month: 'short', year: '2-digit' });
            });
            costData = sortedMonths.map(m => months[m]);
        } else {
            // Daily
            labels = days.map(d => {
                const dt = new Date(d.date + 'T00:00:00');
                return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
            });
            costData = days.map(d => d.cost || 0);
        }

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.timeline = new Chart(canvas, {
            type: 'line',
            data: {
                labels,
                datasets: [{
                    label: 'Cost',
                    data: costData,
                    borderColor: '#2ea87a',
                    backgroundColor: 'rgba(46, 168, 122, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 2,
                    pointHoverRadius: 5,
                }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: function(ctx) { return 'Cost: ' + formatCost(ctx.parsed.y); },
                        },
                    },
                },
                scales: {
                    x: {
                        ticks: { color: textColor, maxRotation: 45 },
                        grid: { display: false },
                    },
                    y: {
                        beginAtZero: true,
                        ticks: { color: textColor, callback: function(v) { return formatCost(v); } },
                        grid: { color: gridColor, drawBorder: false },
                    },
                },
            },
        });
    },

    /* ---- Cost by Model (Pie) ---- */
    renderModelChart(models) {
        const canvas = document.getElementById('cost-model-chart');
        if (!canvas) return;

        if (!models || models.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No model data yet</p></div>';
            return;
        }

        // Models have tokens but cost is 0 from the API. Use tokens as a proxy for cost display.
        const palette = ['#5b6abf', '#2ea87a', '#e5a921', '#d94452', '#3b82c4', '#9b59b6'];
        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';

        this.charts.modelPie = new Chart(canvas, {
            type: 'pie',
            data: {
                labels: models.map(m => m.name || 'Unknown'),
                datasets: [{
                    data: models.map(m => m.tokens || 0),
                    backgroundColor: models.map((_, i) => palette[i % palette.length]),
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
                                return ctx.label + ': ' + formatTokens(ctx.parsed) + ' tokens';
                            },
                        },
                    },
                },
            },
        });
    },

    /* ---- Cost by Project (Horizontal Bar) ---- */
    renderProjectChart(projects) {
        const canvas = document.getElementById('cost-project-chart');
        if (!canvas) return;

        if (!projects || projects.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No project data yet</p></div>';
            return;
        }

        const sorted = [...projects].sort((a, b) => (b.cost || 0) - (a.cost || 0)).slice(0, 10);
        const palette = [
            '#5b6abf', '#2ea87a', '#e5a921', '#d94452', '#3b82c4',
            '#9b59b6', '#e67e22', '#1abc9c', '#e74c3c', '#34495e',
        ];
        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.projectBar = new Chart(canvas, {
            type: 'bar',
            data: {
                labels: sorted.map(p => p.name || p.path || 'Unknown'),
                datasets: [{
                    label: 'Cost',
                    data: sorted.map(p => p.cost || 0),
                    backgroundColor: sorted.map((_, i) => palette[i % palette.length]),
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
                            label: function(ctx) { return formatCost(ctx.parsed.x); },
                        },
                    },
                },
                scales: {
                    x: {
                        ticks: { color: textColor, callback: function(v) { return formatCost(v); } },
                        grid: { color: gridColor, drawBorder: false },
                    },
                    y: {
                        ticks: { color: textColor },
                        grid: { display: false },
                    },
                },
            },
        });
    },

    /* ---- Cache Analysis ---- */
    renderCacheAnalysis(days) {
        const el = document.getElementById('cache-analysis');
        if (!el) return;

        // We need session-level cache data for this analysis
        // Fetch sessions to get cache token data
        API.getSessions({ limit: 200 }).then(data => {
            const sessions = (data && data.sessions) || [];

            let totalInput = 0;
            let totalCacheRead = 0;
            let totalCacheWrite = 0;

            sessions.forEach(s => {
                totalInput += s.total_input_tokens || 0;
                totalCacheRead += s.total_cache_read_tokens || 0;
                totalCacheWrite += s.total_cache_write_tokens || 0;
            });

            if (totalInput === 0 && totalCacheRead === 0) {
                el.innerHTML = '<div class="empty-state"><p>No cache data available</p></div>';
                return;
            }

            const cacheHitRate = totalInput > 0 ? (totalCacheRead / (totalInput + totalCacheRead)) * 100 : 0;

            // Rough savings estimate: cache reads are ~90% cheaper than regular input
            const savingsTokens = totalCacheRead;
            const estimatedSavingsRatio = 0.9; // cache reads save ~90% per token
            // Using a rough price per token (e.g., $3/MTok for sonnet input)
            const roughPricePerMTok = 3.0;
            const estimatedSavings = (savingsTokens / 1_000_000) * roughPricePerMTok * estimatedSavingsRatio;

            el.innerHTML = `
                <div class="card-grid" style="grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));">
                    <div style="text-align:center;">
                        <div style="font-size:2rem;font-weight:700;color:var(--accent-success);">${cacheHitRate.toFixed(1)}%</div>
                        <div class="text-muted text-sm">Cache Hit Rate</div>
                    </div>
                    <div style="text-align:center;">
                        <div style="font-size:2rem;font-weight:700;color:var(--accent-primary);">${formatTokens(totalCacheRead)}</div>
                        <div class="text-muted text-sm">Cache Read Tokens</div>
                    </div>
                    <div style="text-align:center;">
                        <div style="font-size:2rem;font-weight:700;color:var(--accent-warning);">${formatTokens(totalCacheWrite)}</div>
                        <div class="text-muted text-sm">Cache Write Tokens</div>
                    </div>
                    <div style="text-align:center;">
                        <div style="font-size:2rem;font-weight:700;color:var(--accent-success);">~${formatCost(estimatedSavings)}</div>
                        <div class="text-muted text-sm">Estimated Savings</div>
                    </div>
                </div>
                <div class="mt-2">
                    <div style="background:var(--bg-tertiary);border-radius:8px;height:24px;overflow:hidden;position:relative;">
                        <div style="background:var(--accent-success);height:100%;width:${Math.min(cacheHitRate, 100)}%;border-radius:8px;transition:width 0.3s;"></div>
                        <div style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);font-size:0.75rem;font-weight:600;color:var(--text-primary);">
                            ${cacheHitRate.toFixed(1)}% of input tokens served from cache
                        </div>
                    </div>
                </div>
            `;
        }).catch(() => {
            el.innerHTML = '<div class="empty-state"><p>Could not load cache data</p></div>';
        });
    },

    /* ---- Cost Projection ---- */
    renderCostProjection(days) {
        const el = document.getElementById('cost-projection');
        if (!el) return;

        if (!days || days.length < 7) {
            el.innerHTML = '<div class="empty-state"><p>Need at least 7 days of data for projection</p></div>';
            return;
        }

        // Use last 14 days for trend
        const recent = days.slice(-14);
        const recentAvg = recent.reduce((s, d) => s + (d.cost || 0), 0) / recent.length;
        const last7 = days.slice(-7);
        const last7Avg = last7.reduce((s, d) => s + (d.cost || 0), 0) / last7.length;

        const projected30 = last7Avg * 30;
        const projectedWeekly = last7Avg * 7;

        // Trend direction
        const trendPct = recentAvg > 0 ? ((last7Avg - recentAvg) / recentAvg * 100) : 0;
        const trendDir = trendPct > 5 ? 'increasing' : trendPct < -5 ? 'decreasing' : 'stable';
        const trendColor = trendDir === 'increasing' ? 'var(--accent-danger)' : trendDir === 'decreasing' ? 'var(--accent-success)' : 'var(--text-muted)';
        const trendSymbol = trendDir === 'increasing' ? '&#9650;' : trendDir === 'decreasing' ? '&#9660;' : '&#8226;';

        el.innerHTML = `
            <div class="card-grid" style="grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));">
                <div style="text-align:center;">
                    <div style="font-size:1.8rem;font-weight:700;color:var(--accent-primary);">${formatCost(projectedWeekly)}</div>
                    <div class="text-muted text-sm">Projected Weekly</div>
                </div>
                <div style="text-align:center;">
                    <div style="font-size:1.8rem;font-weight:700;color:var(--accent-primary);">${formatCost(projected30)}</div>
                    <div class="text-muted text-sm">Projected Monthly</div>
                </div>
                <div style="text-align:center;">
                    <div style="font-size:1.8rem;font-weight:700;color:var(--accent-primary);">${formatCost(last7Avg)}</div>
                    <div class="text-muted text-sm">Daily Average (7d)</div>
                </div>
                <div style="text-align:center;">
                    <div style="font-size:1.8rem;font-weight:700;color:${trendColor};">${trendSymbol} ${Math.abs(trendPct).toFixed(1)}%</div>
                    <div class="text-muted text-sm">Trend (${trendDir})</div>
                </div>
            </div>
            <div class="text-muted text-sm mt-1">Projection based on 7-day rolling average of ${formatCost(last7Avg)}/day.</div>
        `;
    },

    /* ---- Token Efficiency ---- */
    renderTokenEfficiency(data) {
        if (!data) return;
        const container = document.querySelector('.cost-page');
        if (!container) return;

        const section = document.createElement('div');
        section.className = 'token-efficiency-section mt-2';
        section.innerHTML = `
            <h2 class="mt-2">Token Efficiency</h2>
            <div class="card-grid mt-2" style="grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));">
                <div class="card stat-card" style="margin-bottom:0;">
                    <div class="stat-value" style="color:var(--accent-success);">${((data.cache_hit_rate || 0) * 100).toFixed(1)}%</div>
                    <div class="stat-label">Cache Hit Rate</div>
                </div>
                <div class="card stat-card" style="margin-bottom:0;">
                    <div class="stat-value" style="color:var(--accent-success);">$${(data.cache_savings_usd || 0).toFixed(2)}</div>
                    <div class="stat-label">Cache Savings</div>
                </div>
                <div class="card stat-card" style="margin-bottom:0;">
                    <div class="stat-value">${Math.round(data.avg_tokens_per_tool_call || 0).toLocaleString()}</div>
                    <div class="stat-label">Avg Tokens/Tool Call</div>
                </div>
                <div class="card stat-card" style="margin-bottom:0;">
                    <div class="stat-value">${Math.round(data.avg_output_tokens_per_message || 0).toLocaleString()}</div>
                    <div class="stat-label">Avg Output/Message</div>
                </div>
            </div>
            ${data.efficiency_by_model && data.efficiency_by_model.length > 0 ? `
            <div class="card mt-2">
                <div class="card-header"><h3>Efficiency by Model</h3></div>
                <div class="table-wrapper">
                    <table>
                        <thead><tr><th>Model</th><th class="text-right">Cache Hit Rate</th><th class="text-right">Output Ratio</th></tr></thead>
                        <tbody>${data.efficiency_by_model.map(m =>
                            `<tr><td>${this._esc(m.model)}</td><td class="text-right">${(m.cache_hit_rate * 100).toFixed(1)}%</td><td class="text-right">${(m.avg_output_ratio * 100).toFixed(1)}%</td></tr>`
                        ).join('')}</tbody>
                    </table>
                </div>
            </div>` : ''}
        `;
        container.appendChild(section);
    },

    /* ---- Budgets ---- */
    renderBudgets(data) {
        const container = document.querySelector('.cost-page');
        if (!container) return;

        const section = document.createElement('div');
        section.className = 'budget-section mt-2';

        const budgets = (data && data.budgets) || [];

        let tableRows = '';
        if (budgets.length > 0) {
            tableRows = budgets.map(b => {
                const pct = (b.percentage || 0).toFixed(1);
                let barColor = 'var(--accent-success)';
                if (b.status === 'exceeded') barColor = 'var(--accent-danger)';
                else if (b.status === 'warning') barColor = 'var(--accent-warning)';

                return `<tr>
                    <td>${this._esc(b.name)}</td>
                    <td>${this._esc(b.project_path || 'All')}</td>
                    <td>${this._esc(b.period)}</td>
                    <td class="text-right">${formatCost(b.amount_usd)}</td>
                    <td class="text-right">${formatCost(b.current_spend)}</td>
                    <td style="min-width:120px;">
                        <div style="display:flex;align-items:center;gap:8px;">
                            <div style="flex:1;background:var(--bg-tertiary);border-radius:4px;height:8px;overflow:hidden;">
                                <div style="background:${barColor};height:100%;width:${Math.min(parseFloat(pct), 100)}%;border-radius:4px;"></div>
                            </div>
                            <span style="font-size:0.8rem;color:${barColor};font-weight:600;">${pct}%</span>
                        </div>
                    </td>
                    <td>
                        <button class="btn btn-sm btn-danger" onclick="CostPage._deleteBudget(${b.id})" style="padding:2px 8px;font-size:0.75rem;">Delete</button>
                    </td>
                </tr>`;
            }).join('');
        }

        section.innerHTML = `
            <div class="card">
                <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
                    <h3>Budgets</h3>
                    <button class="btn btn-sm" onclick="CostPage._addBudget()" style="padding:4px 12px;">Add Budget</button>
                </div>
                ${budgets.length > 0 ? `
                <div class="table-wrapper">
                    <table>
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Project</th>
                                <th>Period</th>
                                <th class="text-right">Budget</th>
                                <th class="text-right">Spent</th>
                                <th>Progress</th>
                                <th></th>
                            </tr>
                        </thead>
                        <tbody>${tableRows}</tbody>
                    </table>
                </div>` : '<div class="empty-state"><p>No budgets configured. Click "Add Budget" to create one.</p></div>'}
            </div>
        `;
        container.appendChild(section);
    },

    async _addBudget() {
        const name = prompt('Budget name:');
        if (!name) return;
        const period = prompt('Period (daily, weekly, monthly):', 'monthly');
        if (!period || !['daily', 'weekly', 'monthly'].includes(period)) {
            if (typeof App !== 'undefined' && App.toast) App.toast('Invalid period. Use daily, weekly, or monthly.', 'error');
            return;
        }
        const amountStr = prompt('Budget amount (USD):', '100');
        if (!amountStr) return;
        const amount = parseFloat(amountStr);
        if (isNaN(amount) || amount <= 0) {
            if (typeof App !== 'undefined' && App.toast) App.toast('Invalid amount.', 'error');
            return;
        }
        try {
            await API.createBudget({ name, period, amount_usd: amount });
            const container = document.getElementById('main-content');
            if (container) { container.innerHTML = ''; this.render(container); }
        } catch (err) {
            if (typeof App !== 'undefined' && App.toast) App.toast('Failed to create budget: ' + err.message, 'error');
        }
    },

    async _deleteBudget(id) {
        if (!confirm('Delete this budget?')) return;
        try {
            await API.deleteBudget(id);
            const container = document.getElementById('main-content');
            if (container) { container.innerHTML = ''; this.render(container); }
        } catch (err) {
            if (typeof App !== 'undefined' && App.toast) App.toast('Failed to delete budget: ' + err.message, 'error');
        }
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
