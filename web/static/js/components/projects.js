/* ==========================================================================
   Claude Monitor — Projects Page
   ========================================================================== */

const ProjectsPage = {
    charts: {},
    currentView: 'list',
    selectedProject: null,
    sortCol: 'sessions',
    sortDir: 'desc',

    async render(container) {
        if (this.currentView === 'detail' && this.selectedProject) {
            await this.renderDetail(container);
        } else {
            await this.renderList(container);
        }
    },

    /* ------------------------------------------------------------------
       List View
    ------------------------------------------------------------------ */
    async renderList(container) {
        container.innerHTML = `
            <div class="projects-page">
                <h2>Projects</h2>
                <div id="projects-content">
                    <div class="loading-spinner"></div>
                </div>
            </div>
        `;
        await this.loadList();
    },

    async loadList() {
        const area = document.getElementById('projects-content');
        if (!area) return;

        try {
            const data = await API.getProjectStats();
            const projects = (data && data.projects) || [];

            if (projects.length === 0) {
                area.innerHTML = '<div class="empty-state"><h3>No projects found</h3><p>Projects will appear here once Claude Code sessions are tracked.</p></div>';
                return;
            }

            // Sort
            const sorted = [...projects].sort((a, b) => {
                const av = a[this.sortCol] || 0;
                const bv = b[this.sortCol] || 0;
                if (typeof av === 'string') {
                    return this.sortDir === 'asc' ? av.localeCompare(bv) : bv.localeCompare(av);
                }
                return this.sortDir === 'asc' ? av - bv : bv - av;
            });

            const sortIcon = (col) => {
                if (this.sortCol !== col) return '';
                return this.sortDir === 'asc' ? ' &#9650;' : ' &#9660;';
            };

            // Compute totals for token/cost per project
            // The API returns sessions and cost per project
            area.innerHTML = `
                <div class="mb-1 text-muted text-sm">${projects.length} project${projects.length !== 1 ? 's' : ''}</div>
                <div class="table-wrapper">
                    <table>
                        <thead>
                            <tr>
                                <th class="cursor-pointer no-select" data-sort="name">Project${sortIcon('name')}</th>
                                <th>Path</th>
                                <th class="text-right cursor-pointer no-select" data-sort="sessions">Sessions${sortIcon('sessions')}</th>
                                <th class="text-right cursor-pointer no-select" data-sort="cost">Cost${sortIcon('cost')}</th>
                                <th></th>
                            </tr>
                        </thead>
                        <tbody>
                            ${sorted.map(p => `
                                <tr class="cursor-pointer project-row" data-project="${this._esc(p.path || p.name || '')}">
                                    <td class="font-bold">${this._esc(p.name || 'Unknown')}</td>
                                    <td class="truncate text-muted text-sm" style="max-width:300px;" title="${this._esc(p.path || '')}">${this._esc(p.path || '-')}</td>
                                    <td class="text-right">${p.sessions || 0}</td>
                                    <td class="text-right">${formatCost(p.cost)}</td>
                                    <td class="text-right"><span class="badge badge-info" style="cursor:pointer;">Details</span></td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>

                <!-- Top directories heatmap -->
                <div class="card mt-2">
                    <div class="card-header"><h3>Project Activity Distribution</h3></div>
                    <div id="project-heatmap" style="overflow-x:auto;"></div>
                </div>
            `;

            // Bind row clicks
            area.querySelectorAll('.project-row').forEach(row => {
                row.addEventListener('click', () => {
                    this.selectedProject = row.dataset.project;
                    this.currentView = 'detail';
                    const c = document.getElementById('main-content');
                    c.innerHTML = '';
                    this.render(c);
                });
            });

            // Bind column sort
            area.querySelectorAll('th[data-sort]').forEach(th => {
                th.addEventListener('click', (e) => {
                    e.stopPropagation();
                    const col = th.dataset.sort;
                    if (this.sortCol === col) {
                        this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc';
                    } else {
                        this.sortCol = col;
                        this.sortDir = 'desc';
                    }
                    const c = document.getElementById('main-content');
                    c.innerHTML = '';
                    this.render(c);
                });
            });

            // Render activity heatmap
            this.renderProjectHeatmap(sorted);

        } catch (err) {
            console.error('Projects list error:', err);
            area.innerHTML = `<div class="empty-state"><h3>Error loading projects</h3><p>${this._esc(err.message)}</p></div>`;
        }
    },

    /* ---- Project Activity Heatmap (horizontal bars) ---- */
    renderProjectHeatmap(projects) {
        const el = document.getElementById('project-heatmap');
        if (!el) return;

        if (!projects || projects.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No data</p></div>';
            return;
        }

        const maxSessions = Math.max(...projects.map(p => p.sessions || 0));
        const top10 = projects.slice(0, 10);

        el.innerHTML = `
            <div style="padding: 8px 0;">
                ${top10.map(p => {
                    const width = maxSessions > 0 ? Math.max(2, ((p.sessions || 0) / maxSessions) * 100) : 2;
                    const name = p.name || p.path || 'Unknown';
                    return `
                        <div style="display:flex;align-items:center;gap:12px;margin-bottom:6px;">
                            <div class="truncate text-sm" style="width:160px;flex-shrink:0;" title="${this._esc(name)}">${this._esc(name)}</div>
                            <div style="flex:1;background:var(--bg-tertiary);border-radius:4px;height:20px;overflow:hidden;">
                                <div style="width:${width}%;background:var(--accent-primary);height:100%;border-radius:4px;transition:width 0.3s;"></div>
                            </div>
                            <div class="text-sm text-muted" style="width:60px;text-align:right;">${p.sessions || 0} sess.</div>
                        </div>
                    `;
                }).join('')}
            </div>
        `;
    },

    /* ------------------------------------------------------------------
       Detail View
    ------------------------------------------------------------------ */
    async renderDetail(container) {
        this.destroyCharts();

        container.innerHTML = `
            <div class="projects-detail">
                <div class="mb-2">
                    <button id="back-to-projects" class="btn btn-secondary btn-sm">&larr; Back to Projects</button>
                </div>
                <h2>Project: ${this._esc(this.selectedProject)}</h2>
                <div id="project-detail-content">
                    <div class="loading-spinner"></div>
                </div>
            </div>
        `;

        document.getElementById('back-to-projects').addEventListener('click', () => {
            this.currentView = 'list';
            this.selectedProject = null;
            const c = document.getElementById('main-content');
            c.innerHTML = '';
            this.render(c);
        });

        await this.loadDetail();
    },

    async loadDetail() {
        const area = document.getElementById('project-detail-content');
        if (!area) return;

        try {
            const [sessionsData, toolStats, breakdownData] = await Promise.all([
                API.getSessions({ project: this.selectedProject, limit: 200 }),
                API.getToolStats(),
                API.getProjectBreakdown(this.selectedProject).catch(() => null),
            ]);

            const sessions = (sessionsData && sessionsData.sessions) || [];
            const tools = (toolStats && toolStats.tools) || [];
            const breakdown = (breakdownData) || {};

            if (sessions.length === 0) {
                area.innerHTML = '<div class="empty-state"><p>No sessions found for this project</p></div>';
                return;
            }

            // Compute summary stats
            let totalTokens = 0;
            let totalCost = 0;
            sessions.forEach(s => {
                totalTokens += (s.total_input_tokens || 0) + (s.total_output_tokens || 0);
                totalCost += s.estimated_cost_usd || 0;
            });

            area.innerHTML = `
                <!-- Summary Cards -->
                <div class="card-grid mt-2">
                    <div class="card stat-card"><div class="stat-value">${sessions.length}</div><div class="stat-label">Sessions</div></div>
                    <div class="card stat-card"><div class="stat-value">${formatTokens(totalTokens)}</div><div class="stat-label">Total Tokens</div></div>
                    <div class="card stat-card"><div class="stat-value">${formatCost(totalCost)}</div><div class="stat-label">Total Cost</div></div>
                </div>

                <!-- Breakdown: Tools, Skills, MCP Servers, Agents -->
                <div id="project-breakdown-section"></div>

                <!-- Charts -->
                <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Sessions Over Time</h3></div>
                        <div class="chart-container" style="max-height:280px;">
                            <canvas id="project-sessions-chart"></canvas>
                        </div>
                    </div>
                    <div class="card" style="margin-bottom:0;">
                        <div class="card-header"><h3>Token Usage</h3></div>
                        <div class="chart-container" style="max-height:280px;">
                            <canvas id="project-tokens-chart"></canvas>
                        </div>
                    </div>
                </div>

                <!-- Sessions Table -->
                <div class="card mt-2">
                    <div class="card-header"><h3>Sessions</h3></div>
                    <div id="project-sessions-table"></div>
                </div>
            `;

            this.renderBreakdownSection(breakdown);
            this.renderDetailSessionsChart(sessions);
            this.renderDetailTokensChart(sessions);
            this.renderDetailSessionsTable(sessions);

            // Add after this.renderDetailSessionsTable(sessions);
            const heatmap = await API.getFileHeatmap(this.selectedProject).catch(() => null);
            if (heatmap && heatmap.files && heatmap.files.length > 0) {
                const maxTotal = Math.max(...heatmap.files.map(f => f.total));
                const fileHtml = `
                <div class="card mt-2">
                    <div class="card-header"><h3>File Activity Heatmap</h3></div>
                    <div class="table-wrapper">
                        <table>
                            <thead><tr><th>File</th><th class="text-right">Reads</th><th class="text-right">Writes</th><th class="text-right">Edits</th><th class="text-right">Total</th><th style="width:100px"></th></tr></thead>
                            <tbody>${heatmap.files.slice(0, 20).map(f => `
                                <tr>
                                    <td class="truncate font-mono text-sm" style="max-width:350px;" title="${this._esc(f.path)}">${this._esc(f.path)}</td>
                                    <td class="text-right">${f.reads}</td>
                                    <td class="text-right">${f.writes}</td>
                                    <td class="text-right">${f.edits}</td>
                                    <td class="text-right"><strong>${f.total}</strong></td>
                                    <td style="width:100px">
                                        <div style="background:var(--bg-secondary);border-radius:2px;height:12px;overflow:hidden">
                                            <div style="height:100%;width:${(f.total/maxTotal*100).toFixed(0)}%;background:linear-gradient(90deg,#3b82f6 ${(f.reads/f.total*100).toFixed(0)}%,#f59e0b ${(f.reads/f.total*100).toFixed(0)}%,#f59e0b ${((f.reads+f.writes)/f.total*100).toFixed(0)}%,#22c55e ${((f.reads+f.writes)/f.total*100).toFixed(0)}%)"></div>
                                        </div>
                                    </td>
                                </tr>
                            `).join('')}</tbody>
                        </table>
                    </div>
                    <div style="display:flex;gap:1rem;font-size:0.75rem;color:var(--text-secondary);margin-top:0.5rem;padding:0 1rem 0.5rem">
                        <span style="color:#3b82f6">&#9632; Reads</span>
                        <span style="color:#f59e0b">&#9632; Writes</span>
                        <span style="color:#22c55e">&#9632; Edits</span>
                    </div>
                </div>`;
                area.insertAdjacentHTML('beforeend', fileHtml);
            }

        } catch (err) {
            console.error('Project detail error:', err);
            area.innerHTML = `<div class="empty-state"><h3>Error</h3><p>${this._esc(err.message)}</p></div>`;
        }
    },

    renderBreakdownSection(breakdown) {
        const el = document.getElementById('project-breakdown-section');
        if (!el) return;

        if (!breakdown || (
            !breakdown.tools?.length &&
            !breakdown.skills?.length &&
            !breakdown.mcp_servers?.length &&
            !breakdown.agents?.length
        )) {
            el.innerHTML = '';
            return;
        }

        const bdTools = breakdown.tools || [];
        const bdSkills = breakdown.skills || [];
        const bdServers = breakdown.mcp_servers || [];
        const bdAgents = breakdown.agents || [];

        // --- Tools Used: top 10 mini horizontal bar chart ---
        let toolsHtml = '';
        if (bdTools.length === 0) {
            toolsHtml = '<div class="text-muted text-sm">No tools used</div>';
        } else {
            const top10 = bdTools.slice(0, 10);
            const maxCount = Math.max(...top10.map(t => t.count || 0));
            toolsHtml = `<div style="padding: 8px 0;">
                ${top10.map(t => {
                    const width = maxCount > 0 ? Math.max(2, ((t.count || 0) / maxCount) * 100) : 2;
                    return `
                        <div style="display:flex;align-items:center;gap:12px;margin-bottom:6px;">
                            <div class="truncate text-sm" style="width:160px;flex-shrink:0;" title="${this._esc(t.name)}">${this._esc(t.name)}</div>
                            <div style="flex:1;background:var(--bg-tertiary);border-radius:4px;height:20px;overflow:hidden;">
                                <div style="width:${width}%;background:var(--accent-primary);height:100%;border-radius:4px;transition:width 0.3s;"></div>
                            </div>
                            <div class="text-sm text-muted" style="width:60px;text-align:right;">${t.count || 0}</div>
                        </div>
                    `;
                }).join('')}
            </div>`;
        }

        // --- Skills Used: badges ---
        let skillsHtml = '';
        if (bdSkills.length === 0) {
            skillsHtml = '<div class="text-muted text-sm">No skills used</div>';
        } else {
            skillsHtml = `<div style="display:flex;flex-wrap:wrap;gap:8px;">
                ${bdSkills.map(s => `<span class="badge badge-info">${this._esc(s.name)} (${s.count || 0})</span>`).join('')}
            </div>`;
        }

        // --- MCP Servers: badges with tooltips ---
        let serversHtml = '';
        if (bdServers.length === 0) {
            serversHtml = '<div class="text-muted text-sm">No MCP servers used</div>';
        } else {
            serversHtml = `<div style="display:flex;flex-wrap:wrap;gap:8px;">
                ${bdServers.map(s => {
                    const serverTools = (s.tools || []).map(t => `${this._esc(t.name)} (${t.count || 0})`).join(', ');
                    const tooltip = serverTools ? serverTools : 'No tools';
                    return `<span class="badge badge-success" title="${this._esc(tooltip)}">${this._esc(s.name)} (${s.count || 0})</span>`;
                }).join('')}
            </div>`;
        }

        // --- Agents: badges ---
        let agentsHtml = '';
        if (bdAgents.length === 0) {
            agentsHtml = '<div class="text-muted text-sm">No agents spawned</div>';
        } else {
            agentsHtml = `<div style="display:flex;flex-wrap:wrap;gap:8px;">
                ${bdAgents.map(a => `<span class="badge badge-info">${this._esc(a.agent_type)} (${a.count || 0})</span>`).join('')}
            </div>`;
        }

        el.innerHTML = `
            <div class="card-grid mt-2" style="grid-template-columns: 1fr 1fr;">
                <div class="card" style="margin-bottom:0;">
                    <div class="card-header"><h3>Tools Used</h3></div>
                    <div style="padding:0 12px 12px;">${toolsHtml}</div>
                </div>
                <div class="card" style="margin-bottom:0;">
                    <div class="card-header"><h3>Skills Used</h3></div>
                    <div style="padding:12px;">${skillsHtml}</div>
                </div>
                <div class="card" style="margin-bottom:0;">
                    <div class="card-header"><h3>MCP Servers</h3></div>
                    <div style="padding:12px;">${serversHtml}</div>
                </div>
                <div class="card" style="margin-bottom:0;">
                    <div class="card-header"><h3>Agents</h3></div>
                    <div style="padding:12px;">${agentsHtml}</div>
                </div>
            </div>
        `;
    },

    renderDetailSessionsChart(sessions) {
        const canvas = document.getElementById('project-sessions-chart');
        if (!canvas) return;

        // Group sessions by day
        const dayCounts = {};
        sessions.forEach(s => {
            if (!s.started_at) return;
            const day = s.started_at.slice(0, 10);
            dayCounts[day] = (dayCounts[day] || 0) + 1;
        });

        const sortedDays = Object.keys(dayCounts).sort().slice(-30);

        if (sortedDays.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No data</p></div>';
            return;
        }

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.sessionsLine = new Chart(canvas, {
            type: 'line',
            data: {
                labels: sortedDays.map(d => {
                    const dt = new Date(d + 'T00:00:00');
                    return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
                }),
                datasets: [{
                    label: 'Sessions',
                    data: sortedDays.map(d => dayCounts[d] || 0),
                    borderColor: '#5b6abf',
                    backgroundColor: 'rgba(91, 106, 191, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 3,
                }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: {
                    x: { ticks: { color: textColor, maxRotation: 45 }, grid: { display: false } },
                    y: { beginAtZero: true, ticks: { color: textColor, stepSize: 1 }, grid: { color: gridColor, drawBorder: false } },
                },
            },
        });
    },

    renderDetailTokensChart(sessions) {
        const canvas = document.getElementById('project-tokens-chart');
        if (!canvas) return;

        // Group tokens by day
        const dayTokens = {};
        sessions.forEach(s => {
            if (!s.started_at) return;
            const day = s.started_at.slice(0, 10);
            if (!dayTokens[day]) dayTokens[day] = { input: 0, output: 0 };
            dayTokens[day].input += s.total_input_tokens || 0;
            dayTokens[day].output += s.total_output_tokens || 0;
        });

        const sortedDays = Object.keys(dayTokens).sort().slice(-30);

        if (sortedDays.length === 0) {
            canvas.parentElement.innerHTML = '<div class="empty-state"><p>No data</p></div>';
            return;
        }

        const textColor = getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() || '#888';
        const gridColor = getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() || '#e0e0e8';

        this.charts.tokensBar = new Chart(canvas, {
            type: 'bar',
            data: {
                labels: sortedDays.map(d => {
                    const dt = new Date(d + 'T00:00:00');
                    return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
                }),
                datasets: [
                    {
                        label: 'Input',
                        data: sortedDays.map(d => dayTokens[d].input),
                        backgroundColor: '#5b6abf',
                        borderRadius: 2,
                    },
                    {
                        label: 'Output',
                        data: sortedDays.map(d => dayTokens[d].output),
                        backgroundColor: '#2ea87a',
                        borderRadius: 2,
                    },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { labels: { color: textColor, usePointStyle: true } },
                    tooltip: {
                        callbacks: {
                            label: function(ctx) { return ctx.dataset.label + ': ' + formatTokens(ctx.parsed.y); },
                        },
                    },
                },
                scales: {
                    x: { stacked: true, ticks: { color: textColor, maxRotation: 45 }, grid: { display: false } },
                    y: {
                        stacked: true,
                        ticks: { color: textColor, callback: function(v) { return formatTokens(v); } },
                        grid: { color: gridColor, drawBorder: false },
                    },
                },
            },
        });
    },

    renderDetailSessionsTable(sessions) {
        const el = document.getElementById('project-sessions-table');
        if (!el) return;

        const sorted = [...sessions].sort((a, b) => (b.started_at || '').localeCompare(a.started_at || ''));
        const display = sorted.slice(0, 50);

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr>
                            <th>Date/Time</th>
                            <th>Duration</th>
                            <th>Branch</th>
                            <th class="text-right">Input Tokens</th>
                            <th class="text-right">Output Tokens</th>
                            <th class="text-right">Cost</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${display.map(s => `
                            <tr class="cursor-pointer session-row" data-session-id="${this._esc(s.id)}">
                                <td style="white-space:nowrap;">${formatDate(s.started_at)}</td>
                                <td>${formatDuration(s.started_at, s.ended_at)}</td>
                                <td class="truncate" style="max-width:120px;" title="${this._esc(s.git_branch || '')}">${this._esc(s.git_branch || '-')}</td>
                                <td class="text-right">${formatTokens(s.total_input_tokens)}</td>
                                <td class="text-right">${formatTokens(s.total_output_tokens)}</td>
                                <td class="text-right">${formatCost(s.estimated_cost_usd)}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
            ${sessions.length > 50 ? `<div class="text-muted text-sm mt-1">Showing 50 of ${sessions.length} sessions</div>` : ''}
        `;

        el.querySelectorAll('.session-row').forEach(row => {
            row.addEventListener('click', () => {
                const sid = row.dataset.sessionId;
                if (typeof SessionsPage !== 'undefined') {
                    SessionsPage.selectedSession = sid;
                    SessionsPage.currentView = 'detail';
                }
                window.location.hash = 'sessions';
            });
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
