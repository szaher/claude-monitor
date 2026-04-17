/* ==========================================================================
   Claude Monitor — Live Page
   ========================================================================== */

const LivePage = {
    ws: null,
    paused: false,
    events: [],
    maxEvents: 500,
    reconnectTimer: null,
    reconnectDelay: 3000,
    activeSessionsTimer: null,

    render(container) {
        // Clean up any existing connection before re-rendering
        this.destroy();

        container.innerHTML = `
            <div class="live-page">
                <div class="live-header flex items-center justify-between mb-2">
                    <h2 style="margin-bottom:0;">Live Activity</h2>
                    <div class="flex items-center gap-1">
                        <span id="live-status" class="badge badge-neutral">Connecting...</span>
                        <button id="live-toggle" class="btn btn-secondary btn-sm">Pause</button>
                        <button id="live-clear" class="btn btn-secondary btn-sm">Clear</button>
                    </div>
                </div>
                <div id="live-active-sessions"></div>
                <div id="live-events" class="live-events"></div>
            </div>
        `;

        this.setupControls();
        this.connect();
        this.loadActiveSessions();
        this.activeSessionsTimer = setInterval(() => this.loadActiveSessions(), 15000);
    },

    /* ------------------------------------------------------------------
       WebSocket Connection
    ------------------------------------------------------------------ */
    connect() {
        if (this.ws) {
            try { this.ws.close(); } catch (_) { /* noop */ }
        }

        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.ws = new WebSocket(`${protocol}//${location.host}/ws`);

        this.ws.onopen = () => {
            this.updateStatus('connected');
            if (this.reconnectTimer) {
                clearTimeout(this.reconnectTimer);
                this.reconnectTimer = null;
            }
        };

        this.ws.onclose = () => {
            this.updateStatus('disconnected');
            this.scheduleReconnect();
        };

        this.ws.onerror = () => {
            this.updateStatus('disconnected');
        };

        this.ws.onmessage = (e) => {
            if (this.paused) return;
            try {
                const event = JSON.parse(e.data);
                this.addEvent(event);
            } catch (err) {
                console.warn('Live: failed to parse event:', err);
            }
        };
    },

    scheduleReconnect() {
        if (this.reconnectTimer) return;
        this.reconnectTimer = setTimeout(() => {
            this.reconnectTimer = null;
            // Only reconnect if we're still on the live page
            if (document.getElementById('live-events')) {
                this.connect();
            }
        }, this.reconnectDelay);
    },

    updateStatus(state) {
        const badge = document.getElementById('live-status');
        if (!badge) return;

        if (state === 'connected') {
            badge.className = 'badge badge-success';
            badge.textContent = 'Connected';
        } else {
            badge.className = 'badge badge-danger';
            badge.textContent = 'Disconnected';
        }
    },

    /* ------------------------------------------------------------------
       Active Sessions
    ------------------------------------------------------------------ */
    async loadActiveSessions() {
        const container = document.getElementById('live-active-sessions');
        if (!container) return;

        try {
            const data = await API.getActiveSessions();
            const sessions = (data && data.sessions) || [];
            if (sessions.length === 0) {
                container.innerHTML = '';
                return;
            }

            container.innerHTML = `
                <div class="card mb-2" style="border-left: 3px solid var(--accent-success);">
                    <div class="card-header">
                        <h3 style="margin:0;display:flex;align-items:center;gap:0.5rem;">
                            <span style="color:var(--accent-success);animation:pulse-glow 2s ease-in-out infinite">&#9679;</span>
                            ${sessions.length} Active Session${sessions.length !== 1 ? 's' : ''}
                        </h3>
                    </div>
                    <div class="table-wrapper">
                        <table>
                            <thead>
                                <tr>
                                    <th>Session</th>
                                    <th>Project</th>
                                    <th>Started</th>
                                    <th>Last Activity</th>
                                    <th class="text-right">Tool Calls (15m)</th>
                                    <th class="text-right">Tokens</th>
                                    <th class="text-right">Cost</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${sessions.map(s => {
                                    const totalTokens = (s.total_input_tokens || 0) + (s.total_output_tokens || 0);
                                    return `
                                        <tr class="cursor-pointer" onclick="window.location.hash='sessions'">
                                            <td class="font-mono text-sm">${this._esc((s.id || '').slice(0, 8))}</td>
                                            <td title="${this._esc(s.project_path || '')}">${this._esc(s.project_name || '-')}</td>
                                            <td style="white-space:nowrap">${this._formatTime(s.started_at)}</td>
                                            <td style="white-space:nowrap">${this._formatTime(s.last_activity)}</td>
                                            <td class="text-right">${s.recent_tool_calls || 0}</td>
                                            <td class="text-right">${this._formatCompact(totalTokens)}</td>
                                            <td class="text-right">$${(s.estimated_cost_usd || 0).toFixed(2)}</td>
                                        </tr>
                                    `;
                                }).join('')}
                            </tbody>
                        </table>
                    </div>
                </div>
            `;
        } catch (err) {
            console.warn('Failed to load active sessions:', err);
        }
    },

    _formatCompact(n) {
        if (n == null || isNaN(n)) return '0';
        n = Number(n);
        if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
        if (n >= 1_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'K';
        return String(n);
    },

    /* ------------------------------------------------------------------
       Controls
    ------------------------------------------------------------------ */
    setupControls() {
        const toggleBtn = document.getElementById('live-toggle');
        const clearBtn = document.getElementById('live-clear');

        if (toggleBtn) {
            toggleBtn.addEventListener('click', () => {
                this.paused = !this.paused;
                toggleBtn.textContent = this.paused ? 'Resume' : 'Pause';
                toggleBtn.className = this.paused
                    ? 'btn btn-primary btn-sm'
                    : 'btn btn-secondary btn-sm';
            });
        }

        if (clearBtn) {
            clearBtn.addEventListener('click', () => {
                this.events = [];
                const eventsContainer = document.getElementById('live-events');
                if (eventsContainer) {
                    eventsContainer.innerHTML = `
                        <div class="empty-state">
                            <p>No events yet. Waiting for activity...</p>
                        </div>
                    `;
                }
            });
        }
    },

    /* ------------------------------------------------------------------
       Event Handling
    ------------------------------------------------------------------ */
    addEvent(event) {
        this.events.unshift(event);
        if (this.events.length > this.maxEvents) {
            this.events.pop();
        }
        this.renderEvent(event);
    },

    renderEvent(event) {
        const container = document.getElementById('live-events');
        if (!container) return;

        // Remove empty state if present
        const emptyState = container.querySelector('.empty-state');
        if (emptyState) {
            emptyState.remove();
        }

        const el = document.createElement('div');
        const eventType = this.getEventType(event);
        el.className = `live-event event-${eventType}`;
        el.innerHTML = this.formatEvent(event);
        container.prepend(el);

        // Remove excess DOM elements
        while (container.children.length > this.maxEvents) {
            container.removeChild(container.lastChild);
        }
    },

    formatEvent(event) {
        const eventName = event.hook_event_name || event.event || event.type || 'unknown';
        const sessionID = event.session_id || '';
        const shortSession = sessionID ? sessionID.slice(0, 8) : '';
        const ts = this._formatTime(event.timestamp || new Date().toISOString());

        let detail = '';

        switch (eventName) {
            case 'SessionStart': {
                const cwd = event.cwd || '';
                const project = cwd ? cwd.split('/').pop() : 'unknown';
                detail = `Session started in <strong>${this._esc(project)}</strong>`;
                if (cwd) {
                    detail += ` <span class="text-muted text-sm">${this._esc(cwd)}</span>`;
                }
                break;
            }

            case 'SessionEnd':
            case 'Stop':
                detail = 'Session ended';
                if (event.stop_reason) {
                    detail += ` (${this._esc(event.stop_reason)})`;
                }
                break;

            case 'PreToolUse': {
                const toolName = event.tool_name || 'unknown';
                detail = `Tool queued: <strong>${this._esc(toolName)}</strong>`;
                break;
            }

            case 'PostToolUse': {
                const toolName = event.tool_name || 'unknown';
                let inputSummary = '';
                if (event.tool_input) {
                    try {
                        const parsed = typeof event.tool_input === 'string'
                            ? JSON.parse(event.tool_input)
                            : event.tool_input;
                        inputSummary = this._toolInputSummary(toolName, parsed);
                    } catch (_) {
                        inputSummary = this._truncate(String(event.tool_input), 60);
                    }
                }
                detail = `Tool: <strong>${this._esc(toolName)}</strong>`;
                if (inputSummary) {
                    detail += ` &mdash; <span class="text-muted">${this._esc(inputSummary)}</span>`;
                }
                break;
            }

            case 'SubagentStart': {
                const agentType = event.agent_type || 'unknown';
                const desc = event.description || '';
                detail = `Agent spawned: <strong>${this._esc(agentType)}</strong>`;
                if (desc) {
                    detail += ` &mdash; ${this._esc(this._truncate(desc, 80))}`;
                }
                break;
            }

            case 'SubagentStop': {
                const agentType = event.agent_type || 'agent';
                detail = `Agent completed: <strong>${this._esc(agentType)}</strong>`;
                break;
            }

            case 'Notification': {
                const title = event.title || '';
                const msg = event.message || '';
                detail = `Notification: <strong>${this._esc(title || msg)}</strong>`;
                if (title && msg) {
                    detail += ` &mdash; <span class="text-muted">${this._esc(this._truncate(msg, 80))}</span>`;
                }
                break;
            }

            case 'TaskStart':
            case 'TaskComplete':
            case 'TaskUpdate': {
                const taskDesc = event.description || event.task || '';
                const verb = eventName === 'TaskStart' ? 'started' : eventName === 'TaskComplete' ? 'completed' : 'updated';
                detail = `Task ${verb}`;
                if (taskDesc) {
                    detail += `: <strong>${this._esc(this._truncate(taskDesc, 80))}</strong>`;
                }
                break;
            }

            default: {
                detail = `<strong>${this._esc(eventName)}</strong>`;
                const summary = this._extractEventSummary(event);
                if (summary) {
                    detail += ` &mdash; <span class="text-muted">${this._esc(summary)}</span>`;
                }
                break;
            }
        }

        return `
            <div class="live-event-inner">
                <span class="live-event-time">${ts}</span>
                <span class="live-event-badge badge badge-${this.getBadgeType(eventName)}">${this._esc(this.getShortLabel(eventName))}</span>
                <span class="live-event-detail">${detail}</span>
                ${shortSession ? `<span class="live-event-session text-muted text-sm font-mono">${this._esc(shortSession)}</span>` : ''}
            </div>
        `;
    },

    _extractEventSummary(event) {
        for (const key of ['message', 'description', 'title', 'tool_name', 'reason', 'cwd', 'text']) {
            if (event[key] && typeof event[key] === 'string') {
                return this._truncate(event[key], 80);
            }
        }
        return '';
    },

    getEventType(event) {
        const name = event.hook_event_name || event.event || event.type || '';
        if (name.startsWith('Session') || name === 'Stop') return 'session';
        if (name.includes('Tool')) return 'tool';
        if (name.includes('agent') || name.includes('Agent')) return 'agent';
        if (name.includes('Task')) return 'task';
        if (name.includes('Error') || name.includes('error')) return 'error';
        if (name === 'Notification') return 'notification';
        return 'default';
    },

    getBadgeType(eventName) {
        switch (eventName) {
            case 'SessionStart':
            case 'SessionEnd':
            case 'Stop':
                return 'info';
            case 'PreToolUse':
            case 'PostToolUse':
                return 'success';
            case 'SubagentStart':
            case 'SubagentStop':
                return 'warning';
            case 'Notification':
                return 'info';
            case 'TaskStart':
            case 'TaskComplete':
            case 'TaskUpdate':
                return 'primary';
            default:
                return 'neutral';
        }
    },

    getShortLabel(eventName) {
        switch (eventName) {
            case 'SessionStart': return 'START';
            case 'SessionEnd': return 'END';
            case 'Stop': return 'STOP';
            case 'PreToolUse': return 'TOOL';
            case 'PostToolUse': return 'TOOL';
            case 'SubagentStart': return 'AGENT';
            case 'SubagentStop': return 'AGENT';
            case 'Notification': return 'NOTIFY';
            case 'TaskStart': return 'TASK';
            case 'TaskComplete': return 'TASK';
            case 'TaskUpdate': return 'TASK';
            default: return eventName.slice(0, 8).toUpperCase();
        }
    },

    /* ------------------------------------------------------------------
       Cleanup
    ------------------------------------------------------------------ */
    destroy() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
        if (this.activeSessionsTimer) {
            clearInterval(this.activeSessionsTimer);
            this.activeSessionsTimer = null;
        }
        if (this.ws) {
            try { this.ws.close(); } catch (_) { /* noop */ }
            this.ws = null;
        }
        this.paused = false;
    },

    /* ------------------------------------------------------------------
       Helpers
    ------------------------------------------------------------------ */
    _esc(str) {
        if (str == null) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    },

    _truncate(str, maxLen) {
        if (!str || str.length <= maxLen) return str || '';
        return str.slice(0, maxLen) + '...';
    },

    _formatTime(iso) {
        try {
            const d = new Date(iso);
            if (isNaN(d.getTime())) return '--:--:--';
            return d.toLocaleTimeString('en-US', {
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
                hour12: false,
            });
        } catch (_) {
            return '--:--:--';
        }
    },

    _toolInputSummary(toolName, input) {
        if (!input) return '';
        switch (toolName) {
            case 'Bash': return this._truncate(input.command || '', 60);
            case 'Read': return input.file_path || '';
            case 'Write': return input.file_path || '';
            case 'Edit': return input.file_path || '';
            case 'Glob': return input.pattern || '';
            case 'Grep': return (input.pattern || '') + (input.path ? ' in ' + input.path : '');
            default: {
                const keys = Object.keys(input);
                if (keys.length === 0) return '';
                const first = input[keys[0]];
                return typeof first === 'string' ? this._truncate(first, 60) : '';
            }
        }
    },
};
