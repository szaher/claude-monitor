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
                <div id="live-events" class="live-events"></div>
            </div>
        `;

        this.setupControls();
        this.connect();
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
        const eventName = event.hook_event_name || event.event || 'unknown';
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

            default:
                detail = `Event: <strong>${this._esc(eventName)}</strong>`;
                break;
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

    getEventType(event) {
        const name = event.hook_event_name || event.event || '';
        if (name.startsWith('Session') || name === 'Stop') return 'session';
        if (name.includes('Tool')) return 'tool';
        if (name.includes('agent') || name.includes('Agent')) return 'agent';
        if (name.includes('Error') || name.includes('error')) return 'error';
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
