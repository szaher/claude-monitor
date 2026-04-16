/* ==========================================================================
   Claude Monitor — Sessions Page
   ========================================================================== */

const SessionsPage = {
    currentView: 'list',
    selectedSession: null,
    page: 0,
    perPage: 50,
    sortCol: 'started_at',
    sortDir: 'desc',

    async render(container) {
        if (this.currentView === 'detail' && this.selectedSession) {
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
            <div class="sessions-page">
                <h2>Sessions</h2>
                <div id="sessions-table-area">
                    <div class="loading-spinner"></div>
                </div>
            </div>
        `;
        await this.loadList();
    },

    async loadList() {
        const area = document.getElementById('sessions-table-area');
        if (!area) return;

        try {
            const params = Object.assign({}, App.getFilterParams(), {
                limit: this.perPage,
                offset: this.page * this.perPage,
            });

            const data = await API.getSessions(params);
            const sessions = (data && data.sessions) || [];
            const total = (data && data.total) || 0;

            if (sessions.length === 0 && this.page === 0) {
                area.innerHTML = '<div class="empty-state"><h3>No sessions found</h3><p>Sessions will appear here once Claude Code is used with the monitor hook installed.</p></div>';
                return;
            }

            const totalPages = Math.max(1, Math.ceil(total / this.perPage));

            // Sort icon helper
            const sortIcon = (col) => {
                if (this.sortCol !== col) return '';
                return this.sortDir === 'asc' ? ' &#9650;' : ' &#9660;';
            };

            area.innerHTML = `
                <div class="mb-1 text-muted text-sm">${total} session${total !== 1 ? 's' : ''} total</div>
                <div class="table-wrapper">
                    <table>
                        <thead>
                            <tr>
                                <th class="cursor-pointer no-select" data-sort="started_at">Date/Time${sortIcon('started_at')}</th>
                                <th class="cursor-pointer no-select" data-sort="project_name">Project${sortIcon('project_name')}</th>
                                <th>Duration</th>
                                <th>Model</th>
                                <th class="text-right cursor-pointer no-select" data-sort="total_input_tokens">Input Tokens${sortIcon('total_input_tokens')}</th>
                                <th class="text-right cursor-pointer no-select" data-sort="total_output_tokens">Output Tokens${sortIcon('total_output_tokens')}</th>
                                <th class="text-right">Tool Calls</th>
                                <th class="text-right cursor-pointer no-select" data-sort="estimated_cost_usd">Cost${sortIcon('estimated_cost_usd')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${sessions.map(s => {
                                return `
                                    <tr class="cursor-pointer session-row" data-id="${this._esc(s.id)}">
                                        <td style="white-space:nowrap;">${formatDate(s.started_at)}</td>
                                        <td class="truncate" style="max-width:180px;" title="${this._esc(s.project_path || '')}">${this._esc(s.project_name || '-')}</td>
                                        <td>${formatDuration(s.started_at, s.ended_at)}</td>
                                        <td class="truncate" style="max-width:120px;">${this._esc(s.claude_version || '-')}</td>
                                        <td class="text-right">${formatTokens(s.total_input_tokens)}</td>
                                        <td class="text-right">${formatTokens(s.total_output_tokens)}</td>
                                        <td class="text-right">-</td>
                                        <td class="text-right">${formatCost(s.estimated_cost_usd)}</td>
                                    </tr>
                                `;
                            }).join('')}
                        </tbody>
                    </table>
                </div>
                <div class="pagination mt-2">
                    <button class="btn btn-secondary btn-sm" id="page-prev" ${this.page === 0 ? 'disabled' : ''}>Previous</button>
                    <span class="text-muted text-sm" style="margin: 0 12px;">Page ${this.page + 1} of ${totalPages}</span>
                    <button class="btn btn-secondary btn-sm" id="page-next" ${this.page + 1 >= totalPages ? 'disabled' : ''}>Next</button>
                </div>
            `;

            // Bind row clicks
            area.querySelectorAll('.session-row').forEach(row => {
                row.addEventListener('click', () => {
                    this.selectedSession = row.dataset.id;
                    this.currentView = 'detail';
                    const container = document.getElementById('main-content');
                    container.innerHTML = '';
                    this.render(container);
                });
            });

            // Bind column sort
            area.querySelectorAll('th[data-sort]').forEach(th => {
                th.addEventListener('click', () => {
                    const col = th.dataset.sort;
                    if (this.sortCol === col) {
                        this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc';
                    } else {
                        this.sortCol = col;
                        this.sortDir = 'desc';
                    }
                    this.page = 0;
                    const container = document.getElementById('main-content');
                    container.innerHTML = '';
                    this.render(container);
                });
            });

            // Bind pagination
            const prevBtn = document.getElementById('page-prev');
            const nextBtn = document.getElementById('page-next');
            if (prevBtn) {
                prevBtn.addEventListener('click', () => {
                    if (this.page > 0) {
                        this.page--;
                        const container = document.getElementById('main-content');
                        container.innerHTML = '';
                        this.render(container);
                    }
                });
            }
            if (nextBtn) {
                nextBtn.addEventListener('click', () => {
                    if (this.page + 1 < totalPages) {
                        this.page++;
                        const container = document.getElementById('main-content');
                        container.innerHTML = '';
                        this.render(container);
                    }
                });
            }

        } catch (err) {
            console.error('Sessions list error:', err);
            area.innerHTML = `<div class="empty-state"><h3>Error loading sessions</h3><p>${this._esc(err.message)}</p></div>`;
        }
    },

    /* ------------------------------------------------------------------
       Detail View
    ------------------------------------------------------------------ */
    async renderDetail(container) {
        container.innerHTML = `
            <div class="sessions-detail">
                <div class="mb-2">
                    <button id="back-to-list" class="btn btn-secondary btn-sm">&larr; Back to Sessions</button>
                </div>
                <div id="session-detail-content">
                    <div class="loading-spinner"></div>
                </div>
            </div>
        `;

        document.getElementById('back-to-list').addEventListener('click', () => {
            this.currentView = 'list';
            this.selectedSession = null;
            const c = document.getElementById('main-content');
            c.innerHTML = '';
            this.render(c);
        });

        await this.loadDetail();
    },

    async loadDetail() {
        const area = document.getElementById('session-detail-content');
        if (!area) return;

        try {
            const [data, breakdown] = await Promise.all([
                API.getSession(this.selectedSession),
                API.getSessionBreakdown(this.selectedSession).catch(() => ({})),
            ]);
            if (!data) {
                area.innerHTML = '<div class="empty-state"><h3>Session not found</h3></div>';
                return;
            }

            const session = data.session || {};
            const messages = data.messages || [];
            const toolCalls = data.tool_calls || [];
            const subagents = data.subagents || [];

            const bdTools = breakdown.tools || [];
            const bdSkills = breakdown.skills || [];
            const bdMcp = breakdown.mcp_servers || [];
            const bdAgents = breakdown.agents || [];

            // Build tool call lookup: tool_use_id -> tool_call
            const toolCallMap = {};
            toolCalls.forEach(tc => {
                toolCallMap[tc.id] = tc;
            });

            // Build message-level tool call lookup: message_id -> [tool_calls]
            const messageToolCalls = {};
            toolCalls.forEach(tc => {
                if (!messageToolCalls[tc.message_id]) {
                    messageToolCalls[tc.message_id] = [];
                }
                messageToolCalls[tc.message_id].push(tc);
            });

            const totalTokens = (session.total_input_tokens || 0) + (session.total_output_tokens || 0);

            // Session metadata header
            let headerHTML = `
                <div class="card mb-2">
                    <div class="card-grid" style="grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));">
                        <div>
                            <div class="text-muted text-sm">Project</div>
                            <div class="font-bold">${this._esc(session.project_name || '-')}</div>
                            <div class="text-muted text-sm truncate" style="max-width:250px;" title="${this._esc(session.project_path || '')}">${this._esc(session.project_path || '')}</div>
                        </div>
                        <div>
                            <div class="text-muted text-sm">Branch</div>
                            <div class="font-bold">${this._esc(session.git_branch || '-')}</div>
                        </div>
                        <div>
                            <div class="text-muted text-sm">Start Time</div>
                            <div class="font-bold">${formatDate(session.started_at)}</div>
                        </div>
                        <div>
                            <div class="text-muted text-sm">Duration</div>
                            <div class="font-bold">${formatDuration(session.started_at, session.ended_at)}</div>
                        </div>
                        <div>
                            <div class="text-muted text-sm">Tokens</div>
                            <div class="font-bold">${formatTokens(totalTokens)} <span class="text-muted text-sm">(${formatTokens(session.total_input_tokens || 0)} in / ${formatTokens(session.total_output_tokens || 0)} out)</span></div>
                        </div>
                        <div>
                            <div class="text-muted text-sm">Cost</div>
                            <div class="font-bold">${formatCost(session.estimated_cost_usd)}</div>
                        </div>
                    </div>
                </div>
            `;

            // Breakdown summary cards (2x2 grid)
            headerHTML += `
                <div style="display:grid; grid-template-columns: 1fr 1fr; gap: 12px;" class="mb-2">
                    <div class="card">
                        <div class="card-header"><h3>Tools Used</h3></div>
                        <div class="flex flex-wrap gap-1">
                            ${bdTools.length > 0
                                ? bdTools.map(t => `<span class="badge badge-warning">${this._esc(t.name)} (${t.count})</span>`).join('')
                                : '<span class="text-muted text-sm">None</span>'}
                        </div>
                    </div>
                    <div class="card">
                        <div class="card-header"><h3>Skills Used</h3></div>
                        <div class="flex flex-wrap gap-1">
                            ${bdSkills.length > 0
                                ? bdSkills.map(s => `<span class="badge badge-info">${this._esc(s.name)} (${s.count})</span>`).join('')
                                : '<span class="text-muted text-sm">None</span>'}
                        </div>
                    </div>
                    <div class="card">
                        <div class="card-header"><h3>MCP Servers</h3></div>
                        <div class="flex flex-wrap gap-1">
                            ${bdMcp.length > 0
                                ? bdMcp.map(m => {
                                    const toolTip = (m.tools || []).map(t => t.name + ' (' + t.count + ')').join(', ');
                                    return `<span class="badge badge-success" title="${this._esc(toolTip)}">${this._esc(m.name)} (${m.count})</span>`;
                                }).join('')
                                : '<span class="text-muted text-sm">None</span>'}
                        </div>
                    </div>
                    <div class="card">
                        <div class="card-header"><h3>Agents</h3></div>
                        <div class="flex flex-wrap gap-1">
                            ${bdAgents.length > 0
                                ? bdAgents.map(a => `<span class="badge badge-info">${this._esc(a.agent_type)} (${a.count})</span>`).join('')
                                : (subagents.length > 0
                                    ? subagents.map(sa => `<span class="badge badge-info" title="${this._esc(sa.description || '')}">${this._esc(sa.agent_type || 'agent')}</span>`).join('')
                                    : '<span class="text-muted text-sm">None</span>')}
                        </div>
                    </div>
                </div>
            `;

            // Conversation thread
            let threadHTML = '<div class="conversation-thread">';

            if (messages.length === 0) {
                threadHTML += '<div class="empty-state"><p>No messages in this session</p></div>';
            } else {
                messages.forEach(msg => {
                    threadHTML += this.renderMessage(msg, toolCallMap, messageToolCalls);
                });
            }

            threadHTML += '</div>';

            area.innerHTML = headerHTML + threadHTML;

            // Bind collapsible toggles
            this.bindCollapsibles(area);

        } catch (err) {
            console.error('Session detail error:', err);
            area.innerHTML = `<div class="empty-state"><h3>Error loading session</h3><p>${this._esc(err.message)}</p></div>`;
        }
    },

    /* ------------------------------------------------------------------
       Message Rendering
    ------------------------------------------------------------------ */
    renderMessage(msg, toolCallMap, messageToolCalls) {
        const role = msg.role || msg.type || 'unknown';
        const isUser = role === 'user' || msg.type === 'user';
        const isAssistant = role === 'assistant' || msg.type === 'assistant';

        let html = '';

        if (isUser) {
            html += `
                <div class="msg msg-user">
                    <div class="msg-header">
                        <span class="badge badge-info">User</span>
                        <span class="text-muted text-sm">${formatDate(msg.timestamp)}</span>
                    </div>
                    <div class="msg-body">${this._esc(msg.content_text || '(empty)')}</div>
                </div>
            `;
        } else if (isAssistant) {
            // Parse content_json for structured blocks
            let contentBlocks = null;
            if (msg.content_json) {
                try {
                    contentBlocks = JSON.parse(msg.content_json);
                } catch (_) {
                    contentBlocks = null;
                }
            }

            let tokenInfo = '';
            if (msg.input_tokens || msg.output_tokens) {
                tokenInfo = `<span class="text-muted text-sm ml-1">(${formatTokens(msg.input_tokens || 0)} in / ${formatTokens(msg.output_tokens || 0)} out)</span>`;
            }

            html += `
                <div class="msg msg-assistant">
                    <div class="msg-header">
                        <span class="badge badge-success">Assistant</span>
                        ${msg.model ? `<span class="badge badge-neutral text-sm">${this._esc(msg.model)}</span>` : ''}
                        ${tokenInfo}
                        <span class="text-muted text-sm">${formatDate(msg.timestamp)}</span>
                    </div>
                    <div class="msg-body">
            `;

            if (contentBlocks && Array.isArray(contentBlocks)) {
                contentBlocks.forEach(block => {
                    html += this.renderContentBlock(block, toolCallMap);
                });
            } else if (msg.content_text) {
                html += `<div class="msg-text">${this._esc(msg.content_text)}</div>`;
            } else {
                html += `<div class="text-muted">(empty response)</div>`;
            }

            html += `
                    </div>
                </div>
            `;
        } else {
            // Unknown role
            html += `
                <div class="msg msg-system">
                    <div class="msg-header">
                        <span class="badge badge-neutral">${this._esc(role)}</span>
                        <span class="text-muted text-sm">${formatDate(msg.timestamp)}</span>
                    </div>
                    <div class="msg-body">${this._esc(msg.content_text || '')}</div>
                </div>
            `;
        }

        return html;
    },

    renderContentBlock(block, toolCallMap) {
        if (!block || !block.type) return '';

        switch (block.type) {
            case 'text':
                return `<div class="msg-text">${this._escPre(block.text || '')}</div>`;

            case 'thinking':
                return `
                    <div class="collapsible msg-thinking">
                        <div class="collapsible-header">
                            <span class="collapsible-arrow"></span>
                            <span class="text-muted" style="font-style:italic;">Thinking...</span>
                        </div>
                        <div class="collapsible-body">
                            <div class="collapsible-content" style="font-style:italic; color:var(--text-muted);">
                                ${this._escPre(block.thinking || '')}
                            </div>
                        </div>
                    </div>
                `;

            case 'tool_use': {
                const toolName = block.name || 'Unknown';
                const toolInput = block.input ? JSON.stringify(block.input, null, 2) : '';
                const toolID = block.id || '';

                // Find matching tool_call for response
                const tc = toolCallMap[toolID];
                let responseHTML = '';
                if (tc) {
                    const success = tc.success !== false;
                    const statusBadge = success
                        ? '<span class="badge badge-success">success</span>'
                        : '<span class="badge badge-danger">error</span>';
                    const responseText = tc.tool_response || tc.error || '(no response)';
                    const durationText = tc.duration_ms ? ` (${tc.duration_ms}ms)` : '';

                    responseHTML = `
                        <div class="mt-1">
                            <div class="flex items-center gap-1 mb-1">
                                <strong>Response</strong> ${statusBadge}${durationText ? `<span class="text-muted text-sm">${durationText}</span>` : ''}
                            </div>
                            <pre><code>${this._esc(this._truncate(responseText, 5000))}</code></pre>
                        </div>
                    `;
                }

                return `
                    <div class="collapsible open msg-tool-use">
                        <div class="collapsible-header">
                            <span class="collapsible-arrow"></span>
                            <span class="badge badge-warning">${this._esc(toolName)}</span>
                            <span class="text-muted text-sm">${this._esc(this._toolSummary(toolName, block.input))}</span>
                        </div>
                        <div class="collapsible-body">
                            <div class="collapsible-content">
                                <div>
                                    <strong>Input</strong>
                                    <pre><code>${this._esc(toolInput)}</code></pre>
                                </div>
                                ${responseHTML}
                            </div>
                        </div>
                    </div>
                `;
            }

            case 'tool_result':
                // tool_result blocks are user-side and contain the tool response
                return '';

            default:
                return `<div class="text-muted text-sm">[${this._esc(block.type)} block]</div>`;
        }
    },

    /* ------------------------------------------------------------------
       Collapsible binding
    ------------------------------------------------------------------ */
    bindCollapsibles(container) {
        container.querySelectorAll('.collapsible-header').forEach(header => {
            header.addEventListener('click', () => {
                header.parentElement.classList.toggle('open');
            });
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

    /** Escape and preserve line breaks for display. */
    _escPre(str) {
        if (str == null) return '';
        return this._esc(str).replace(/\n/g, '<br>');
    },

    /** Truncate a string to a maximum length. */
    _truncate(str, maxLen) {
        if (!str || str.length <= maxLen) return str || '';
        return str.slice(0, maxLen) + '... (' + (str.length - maxLen) + ' more chars)';
    },

    /** Generate a short summary of a tool call for the collapsible header. */
    _toolSummary(toolName, input) {
        if (!input) return '';
        try {
            switch (toolName) {
                case 'Bash':
                    return this._truncate(input.command || '', 80);
                case 'Read':
                    return input.file_path || '';
                case 'Write':
                    return input.file_path || '';
                case 'Edit':
                    return input.file_path || '';
                case 'Glob':
                    return input.pattern || '';
                case 'Grep':
                    return (input.pattern || '') + (input.path ? ' in ' + input.path : '');
                default: {
                    const keys = Object.keys(input);
                    if (keys.length === 0) return '';
                    const first = input[keys[0]];
                    if (typeof first === 'string') return this._truncate(first, 60);
                    return '';
                }
            }
        } catch (_) {
            return '';
        }
    },
};
