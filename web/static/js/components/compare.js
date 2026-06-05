/* ==========================================================================
   Claude Monitor — Session Comparison Page
   ========================================================================== */

const ComparePage = {
    sessionA: null,
    sessionB: null,
    data: null,
    sessionsList: null,

    async render(container) {
        const params = new URLSearchParams(window.location.hash.split('?')[1] || '');
        this.sessionA = params.get('a') || this.sessionA;
        this.sessionB = params.get('b') || this.sessionB;

        container.innerHTML = this.template();
        await this.loadSessions();
        this.bindControls();
        if (this.sessionA && this.sessionB) {
            await this.loadComparison();
        }
    },

    template() {
        return `
            <div class="compare-page">
                <h2>Compare Sessions</h2>

                <!-- Session Pickers -->
                <div class="card" style="padding:1rem;margin-bottom:1rem;">
                    <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;">
                        <div style="flex:1;min-width:200px;">
                            <label style="font-size:0.75rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.5px;">Session A</label>
                            <select id="compare-session-a" class="input" style="width:100%;margin-top:4px;">
                                <option value="">Select a session...</option>
                            </select>
                        </div>
                        <button id="compare-swap" class="btn btn-secondary btn-sm" style="margin-top:16px;" title="Swap sessions">&#8644;</button>
                        <div style="flex:1;min-width:200px;">
                            <label style="font-size:0.75rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.5px;">Session B</label>
                            <select id="compare-session-b" class="input" style="width:100%;margin-top:4px;">
                                <option value="">Select a session...</option>
                            </select>
                        </div>
                    </div>
                </div>

                <!-- Comparison Content -->
                <div id="compare-content">
                    <div class="empty-state"><p>Select two sessions to compare</p></div>
                </div>
            </div>
        `;
    },

    async loadSessions() {
        try {
            const result = await API.getSessions({ limit: 50 });
            this.sessionsList = result.sessions || [];
            this.populatePicker('compare-session-a', this.sessionA);
            this.populatePicker('compare-session-b', this.sessionB);
        } catch (_) {}
    },

    populatePicker(selectId, selectedId) {
        const sel = document.getElementById(selectId);
        if (!sel || !this.sessionsList) return;

        this.sessionsList.forEach(s => {
            const opt = document.createElement('option');
            opt.value = s.id;
            const date = formatDate(s.started_at);
            const project = s.project_name || s.project_path || 'unknown';
            const dur = s.ended_at ? formatDuration(s.started_at, s.ended_at) : 'running';
            const cost = formatCost(s.estimated_cost_usd || 0);
            opt.textContent = `${date} — ${project} — ${dur} — ${cost}`;
            if (s.id === selectedId) opt.selected = true;
            sel.appendChild(opt);
        });
    },

    bindControls() {
        const selA = document.getElementById('compare-session-a');
        const selB = document.getElementById('compare-session-b');
        const swap = document.getElementById('compare-swap');

        const onChange = () => {
            this.sessionA = selA.value || null;
            this.sessionB = selB.value || null;
            this.updateURL();
            if (this.sessionA && this.sessionB) {
                this.loadComparison();
            }
        };

        if (selA) selA.addEventListener('change', onChange);
        if (selB) selB.addEventListener('change', onChange);

        if (swap) {
            swap.addEventListener('click', () => {
                const tmp = selA.value;
                selA.value = selB.value;
                selB.value = tmp;
                onChange();
            });
        }
    },

    updateURL() {
        const params = [];
        if (this.sessionA) params.push('a=' + this.sessionA);
        if (this.sessionB) params.push('b=' + this.sessionB);
        const hash = 'compare' + (params.length ? '?' + params.join('&') : '');
        history.replaceState(null, '', '#' + hash);
    },

    async loadComparison() {
        const area = document.getElementById('compare-content');
        if (!area) return;
        area.innerHTML = '<div class="loading-spinner"></div>';

        try {
            this.data = await API.getSessionComparison(this.sessionA, this.sessionB);
            this.renderComparison(area);
        } catch (err) {
            area.innerHTML = `<div class="empty-state"><p>Error: ${err.message}</p></div>`;
        }
    },

    renderComparison(area) {
        if (!this.data) return;
        area.innerHTML = '';

        const metricsCard = document.createElement('div');
        metricsCard.className = 'card mb-2';
        metricsCard.innerHTML = this.renderMetrics();
        area.appendChild(metricsCard);

        const compCard = document.createElement('div');
        compCard.className = 'card mb-2';
        compCard.innerHTML = this.renderComposition();
        area.appendChild(compCard);

        const timeCard = document.createElement('div');
        timeCard.className = 'card mb-2';
        timeCard.innerHTML = this.renderTimeBreakdown();
        area.appendChild(timeCard);
    },

    renderMetrics() {
        const ma = this.data.session_a.metrics;
        const mb = this.data.session_b.metrics;
        const deltas = this.data.deltas;

        const rows = [
            { key: 'cost', label: 'Cost', fmt: 'cost' },
            { key: 'duration_minutes', label: 'Duration', fmt: 'duration' },
            { key: 'total_tokens', label: 'Total Tokens', fmt: 'tokens' },
            { key: 'cache_hit_rate', label: 'Cache Hit Rate', fmt: 'percent' },
            { key: 'tool_calls', label: 'Tool Calls', fmt: 'number' },
            { key: 'error_rate', label: 'Error Rate', fmt: 'percent' },
        ];

        const fmtVal = (v, fmt) => {
            if (v == null) return '-';
            switch (fmt) {
                case 'cost': return formatCost(v);
                case 'duration':
                    if (v < 1) return '< 1m';
                    if (v >= 60) return Math.floor(v / 60) + 'h ' + Math.round(v % 60) + 'm';
                    return Math.round(v) + 'm';
                case 'tokens': return formatTokens(v);
                case 'percent': return (v * 100).toFixed(1) + '%';
                case 'number': return Number(v).toFixed(0);
                default: return String(v);
            }
        };

        const tableRows = rows.map(r => {
            const d = deltas[r.key] || {};
            const pct = d.delta_pct || 0;
            const trend = d.trend || 'stable';
            let color = 'var(--text-muted)';
            if (trend === 'improving') color = '#2ea87a';
            else if (trend === 'worsening') color = '#e85d5d';
            const arrow = pct > 0 ? '↑' : pct < 0 ? '↓' : '';

            return `<tr>
                <td style="font-weight:600;padding:8px 12px;">${r.label}</td>
                <td style="text-align:right;padding:8px 12px;">${fmtVal(ma[r.key], r.fmt)}</td>
                <td style="text-align:right;padding:8px 12px;">${fmtVal(mb[r.key], r.fmt)}</td>
                <td style="text-align:right;padding:8px 12px;color:${color};font-weight:600;">${arrow} ${Math.abs(pct).toFixed(1)}%</td>
            </tr>`;
        }).join('');

        return `
            <div style="padding:1rem;">
                <h3 style="margin:0 0 1rem;">Metrics Comparison</h3>
                <table style="width:100%;border-collapse:collapse;">
                    <thead>
                        <tr style="border-bottom:1px solid var(--border-color);">
                            <th style="text-align:left;padding:8px 12px;color:var(--text-muted);font-size:0.75rem;text-transform:uppercase;">Metric</th>
                            <th style="text-align:right;padding:8px 12px;color:var(--text-muted);font-size:0.75rem;text-transform:uppercase;">Session A</th>
                            <th style="text-align:right;padding:8px 12px;color:var(--text-muted);font-size:0.75rem;text-transform:uppercase;">Session B</th>
                            <th style="text-align:right;padding:8px 12px;color:var(--text-muted);font-size:0.75rem;text-transform:uppercase;">Delta</th>
                        </tr>
                    </thead>
                    <tbody>${tableRows}</tbody>
                </table>
            </div>
        `;
    },

    renderComposition() {
        const bdA = this.data.session_a.breakdown || {};
        const bdB = this.data.session_b.breakdown || {};

        const diffSection = (title, listA, listB, nameKey) => {
            nameKey = nameKey || 'name';
            const namesA = new Set((listA || []).map(x => x[nameKey]));
            const namesB = new Set((listB || []).map(x => x[nameKey]));
            const all = new Set([...namesA, ...namesB]);

            const countMap = (list) => {
                const m = {};
                (list || []).forEach(x => { m[x[nameKey]] = x.count || 0; });
                return m;
            };
            const cA = countMap(listA);
            const cB = countMap(listB);

            if (all.size === 0) return `<div style="margin-bottom:12px;"><strong>${title}:</strong> <span style="color:var(--text-muted);">None</span></div>`;

            const badges = [...all].sort().map(name => {
                const inA = namesA.has(name);
                const inB = namesB.has(name);
                let bg = 'var(--bg-tertiary)';
                let label = name;
                if (inA && inB) {
                    label += ` (${cA[name]} vs ${cB[name]})`;
                } else if (inA) {
                    bg = '#5b6abf33';
                    label += ` (${cA[name]}) — only A`;
                } else {
                    bg = '#f0a03033';
                    label += ` (${cB[name]}) — only B`;
                }
                return `<span class="badge" style="background:${bg};margin:2px;padding:4px 8px;border-radius:4px;font-size:0.8rem;">${label}</span>`;
            }).join('');

            return `<div style="margin-bottom:12px;"><strong>${title}:</strong><div style="margin-top:4px;display:flex;flex-wrap:wrap;gap:4px;">${badges}</div></div>`;
        };

        return `
            <div style="padding:1rem;">
                <h3 style="margin:0 0 1rem;">Composition Comparison</h3>
                ${diffSection('Tools', bdA.tools, bdB.tools, 'name')}
                ${diffSection('Skills', bdA.skills, bdB.skills, 'name')}
                ${diffSection('Agents', bdA.agents, bdB.agents, 'agent_type')}
            </div>
        `;
    },

    renderTimeBreakdown() {
        const segA = this.data.session_a.time_segments || {};
        const segB = this.data.session_b.time_segments || {};

        const renderBar = (label, seg) => {
            const segments = [
                { pct: seg.user_input_pct || 0, color: '#5b6abf', name: 'User input' },
                { pct: seg.assistant_pct || 0, color: '#8b5cf6', name: 'Assistant' },
                { pct: seg.tool_execution_pct || 0, color: '#f0a030', name: 'Tool execution' },
                { pct: seg.other_pct || 0, color: '#7c7c7c', name: 'Other' },
            ];

            const bars = segments
                .filter(s => s.pct > 0)
                .map(s => `<div title="${s.name}: ${s.pct.toFixed(1)}%" style="width:${s.pct}%;background:${s.color};height:100%;display:inline-block;"></div>`)
                .join('');

            return `
                <div style="margin-bottom:12px;">
                    <div style="font-size:0.8rem;font-weight:600;margin-bottom:4px;">${label}</div>
                    <div style="display:flex;height:28px;border-radius:4px;overflow:hidden;background:var(--bg-tertiary);">${bars}</div>
                </div>
            `;
        };

        return `
            <div style="padding:1rem;">
                <h3 style="margin:0 0 1rem;">Time Breakdown</h3>
                ${renderBar('Session A', segA)}
                ${renderBar('Session B', segB)}
                <div style="display:flex;gap:16px;font-size:0.75rem;color:var(--text-muted);margin-top:8px;">
                    <span><span style="display:inline-block;width:10px;height:10px;background:#5b6abf;border-radius:2px;margin-right:4px;"></span>User input</span>
                    <span><span style="display:inline-block;width:10px;height:10px;background:#8b5cf6;border-radius:2px;margin-right:4px;"></span>Assistant</span>
                    <span><span style="display:inline-block;width:10px;height:10px;background:#f0a030;border-radius:2px;margin-right:4px;"></span>Tool execution</span>
                    <span><span style="display:inline-block;width:10px;height:10px;background:#7c7c7c;border-radius:2px;margin-right:4px;"></span>Other</span>
                </div>
            </div>
        `;
    },

    destroy() {},
};
