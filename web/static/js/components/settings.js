/* ==========================================================================
   Claude Monitor — Settings Page
   ========================================================================== */

const SettingsPage = {
    config: null,
    activeTab: 'settings',

    async render(container) {
        container.innerHTML = `
            <div class="settings-page">
                <h2>Settings</h2>
                <div class="tabs mt-2">
                    <button class="tab ${this.activeTab === 'settings' ? 'active' : ''}" data-tab="settings">Settings</button>
                    <button class="tab ${this.activeTab === 'yaml' ? 'active' : ''}" data-tab="yaml">Raw YAML</button>
                </div>
                <div id="settings-content">
                    <div class="loading-spinner"></div>
                </div>
            </div>
        `;

        this.bindTabs();
        await this.loadConfig();
    },

    bindTabs() {
        document.querySelectorAll('.settings-page .tab[data-tab]').forEach(btn => {
            btn.addEventListener('click', () => {
                this.activeTab = btn.dataset.tab;
                document.querySelectorAll('.settings-page .tab[data-tab]').forEach(b => b.classList.toggle('active', b.dataset.tab === this.activeTab));
                this.renderContent();
            });
        });
    },

    async loadConfig() {
        try {
            this.config = await API.getConfig();
            this.renderContent();
        } catch (err) {
            console.error('Settings load error:', err);
            const area = document.getElementById('settings-content');
            if (area) area.innerHTML = `<div class="empty-state"><h3>Error loading config</h3><p>${this._esc(err.message)}</p></div>`;
        }
    },

    renderContent() {
        const area = document.getElementById('settings-content');
        if (!area) return;

        if (!this.config) {
            area.innerHTML = '<div class="empty-state"><p>No configuration loaded</p></div>';
            return;
        }

        if (this.activeTab === 'yaml') {
            this.renderYamlTab(area);
        } else {
            this.renderSettingsTab(area);
        }
    },

    /* ------------------------------------------------------------------
       Settings Tab
    ------------------------------------------------------------------ */
    renderSettingsTab(area) {
        const cfg = this.config;

        area.innerHTML = `
            <!-- Capture Settings: Metadata -->
            <div class="card mt-2">
                <div class="card-header"><h3>Capture - Metadata</h3></div>
                <div class="settings-grid">
                    ${this._toggleRow('capture.metadata.git_branch', 'Git Branch', cfg.capture?.metadata?.git_branch)}
                    ${this._toggleRow('capture.metadata.git_repo', 'Git Repo', cfg.capture?.metadata?.git_repo)}
                    ${this._toggleRow('capture.metadata.working_directory', 'Working Directory', cfg.capture?.metadata?.working_directory)}
                    ${this._toggleRow('capture.metadata.claude_version', 'Claude Version', cfg.capture?.metadata?.claude_version)}
                    ${this._toggleRow('capture.metadata.environment_vars', 'Environment Vars', cfg.capture?.metadata?.environment_vars)}
                    ${this._toggleRow('capture.metadata.command_args', 'Command Args', cfg.capture?.metadata?.command_args)}
                    ${this._toggleRow('capture.metadata.system_info', 'System Info', cfg.capture?.metadata?.system_info)}
                </div>
            </div>

            <!-- Capture Settings: Events -->
            <div class="card mt-2">
                <div class="card-header"><h3>Capture - Events</h3></div>
                <div class="settings-grid">
                    ${this._toggleRow('capture.events.session_start', 'Session Start', cfg.capture?.events?.session_start)}
                    ${this._toggleRow('capture.events.session_end', 'Session End', cfg.capture?.events?.session_end)}
                    ${this._toggleRow('capture.events.pre_tool_use', 'Pre Tool Use', cfg.capture?.events?.pre_tool_use)}
                    ${this._toggleRow('capture.events.post_tool_use', 'Post Tool Use', cfg.capture?.events?.post_tool_use)}
                    ${this._toggleRow('capture.events.subagent_start', 'Subagent Start', cfg.capture?.events?.subagent_start)}
                    ${this._toggleRow('capture.events.subagent_stop', 'Subagent Stop', cfg.capture?.events?.subagent_stop)}
                    ${this._toggleRow('capture.events.stop', 'Stop', cfg.capture?.events?.stop)}
                </div>
            </div>

            <!-- Storage Settings -->
            <div class="card mt-2">
                <div class="card-header"><h3>Storage</h3></div>
                <div class="settings-grid">
                    ${this._inputRow('storage.retention_days', 'Retention Days', cfg.storage?.retention_days || 0, 'number', '0 = unlimited')}
                    ${this._toggleRow('storage.archive_enabled', 'Archive Enabled', cfg.storage?.archive_enabled)}
                    ${this._inputRow('storage.max_db_size_mb', 'Max DB Size (MB)', cfg.storage?.max_db_size_mb || 0, 'number', '0 = unlimited')}
                </div>
            </div>

            <!-- Server Settings -->
            <div class="card mt-2">
                <div class="card-header"><h3>Server</h3></div>
                <div class="settings-grid">
                    ${this._inputRow('server.host', 'Host', cfg.server?.host || '127.0.0.1', 'text')}
                    ${this._inputRow('server.port', 'Port', cfg.server?.port || 3000, 'number')}
                </div>
            </div>

            <!-- UI Settings -->
            <div class="card mt-2">
                <div class="card-header"><h3>UI</h3></div>
                <div class="settings-grid">
                    <div class="settings-row">
                        <div class="settings-label">Theme</div>
                        <div class="settings-control">
                            <select class="input" id="setting-ui.theme" data-path="ui.theme">
                                <option value="auto" ${(cfg.ui?.theme || 'auto') === 'auto' ? 'selected' : ''}>Auto</option>
                                <option value="light" ${cfg.ui?.theme === 'light' ? 'selected' : ''}>Light</option>
                                <option value="dark" ${cfg.ui?.theme === 'dark' ? 'selected' : ''}>Dark</option>
                            </select>
                        </div>
                    </div>
                    ${this._inputRow('ui.sessions_per_page', 'Sessions Per Page', cfg.ui?.sessions_per_page || 50, 'number')}
                </div>
            </div>

            <!-- Model Pricing -->
            <div class="card mt-2">
                <div class="card-header">
                    <h3>Model Pricing ($ per million tokens)</h3>
                    <button class="btn btn-secondary btn-sm" id="add-model-btn">+ Add Model</button>
                </div>
                <div id="model-pricing-table"></div>
            </div>

            <!-- Save Button -->
            <div class="mt-2 flex gap-1">
                <button class="btn btn-primary" id="save-settings-btn">Save Settings</button>
                <button class="btn btn-secondary" id="reset-settings-btn">Reset to Defaults</button>
            </div>
        `;

        this.renderModelPricingTable();
        this.bindThemeChange();
        this.bindSaveButton();
        this.bindResetButton();
        this.bindAddModelButton();
    },

    _toggleRow(path, label, value) {
        const checked = value ? 'checked' : '';
        return `
            <div class="settings-row">
                <div class="settings-label">${this._esc(label)}</div>
                <div class="settings-control">
                    <label class="toggle-switch">
                        <input type="checkbox" data-path="${this._esc(path)}" ${checked}>
                        <span class="toggle-slider"></span>
                    </label>
                </div>
            </div>
        `;
    },

    _inputRow(path, label, value, type, placeholder) {
        return `
            <div class="settings-row">
                <div class="settings-label">${this._esc(label)}</div>
                <div class="settings-control">
                    <input type="${type}" class="input" data-path="${this._esc(path)}" value="${this._esc(String(value))}" ${placeholder ? 'placeholder="' + this._esc(placeholder) + '"' : ''} style="width:200px;">
                </div>
            </div>
        `;
    },

    renderModelPricingTable() {
        const el = document.getElementById('model-pricing-table');
        if (!el) return;

        const models = this.config.cost?.models || {};
        const entries = Object.entries(models);

        if (entries.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No models configured</p></div>';
            return;
        }

        el.innerHTML = `
            <div class="table-wrapper">
                <table>
                    <thead>
                        <tr>
                            <th>Model</th>
                            <th class="text-right">Input</th>
                            <th class="text-right">Output</th>
                            <th class="text-right">Cache Read</th>
                            <th class="text-right">Cache Write</th>
                            <th></th>
                        </tr>
                    </thead>
                    <tbody>
                        ${entries.map(([name, pricing]) => `
                            <tr data-model="${this._esc(name)}">
                                <td><code>${this._esc(name)}</code></td>
                                <td class="text-right"><input type="number" step="0.01" class="input" data-model-field="input" value="${pricing.input || 0}" style="width:90px;text-align:right;"></td>
                                <td class="text-right"><input type="number" step="0.01" class="input" data-model-field="output" value="${pricing.output || 0}" style="width:90px;text-align:right;"></td>
                                <td class="text-right"><input type="number" step="0.01" class="input" data-model-field="cache_read" value="${pricing.cache_read || 0}" style="width:90px;text-align:right;"></td>
                                <td class="text-right"><input type="number" step="0.01" class="input" data-model-field="cache_write" value="${pricing.cache_write || 0}" style="width:90px;text-align:right;"></td>
                                <td class="text-right"><button class="btn btn-danger btn-sm remove-model-btn" data-model-name="${this._esc(name)}">Remove</button></td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;

        // Bind remove buttons
        el.querySelectorAll('.remove-model-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const name = btn.dataset.modelName;
                if (this.config.cost && this.config.cost.models) {
                    delete this.config.cost.models[name];
                }
                this.renderModelPricingTable();
            });
        });
    },

    bindThemeChange() {
        const themeSelect = document.getElementById('setting-ui.theme');
        if (!themeSelect) return;

        themeSelect.addEventListener('change', () => {
            App.applyTheme(themeSelect.value);
        });
    },

    bindSaveButton() {
        const btn = document.getElementById('save-settings-btn');
        if (!btn) return;

        btn.addEventListener('click', async () => {
            try {
                this.collectFormValues();
                const result = await API.saveConfig(this.config);
                this.config = result;
                App.toast('Settings saved successfully', 'success');
            } catch (err) {
                console.error('Save settings error:', err);
                App.toast('Failed to save settings: ' + err.message, 'error');
            }
        });
    },

    bindResetButton() {
        const btn = document.getElementById('reset-settings-btn');
        if (!btn) return;

        btn.addEventListener('click', async () => {
            if (!confirm('Reset all settings to defaults? This cannot be undone.')) return;
            try {
                // Post a default config
                const defaultCfg = {
                    server: { port: 3000, host: '127.0.0.1' },
                    capture: {
                        metadata: {
                            git_branch: true, git_repo: true, working_directory: true,
                            claude_version: true, environment_vars: false, command_args: false, system_info: true,
                        },
                        events: {
                            session_start: true, session_end: true, pre_tool_use: true,
                            post_tool_use: true, subagent_start: true, subagent_stop: true, stop: true,
                        },
                    },
                    storage: { archive_enabled: false, retention_days: 0, max_db_size_mb: 0 },
                    cost: {
                        models: {
                            opus: { input: 15.0, output: 75.0, cache_read: 1.5, cache_write: 18.75 },
                            sonnet: { input: 3.0, output: 15.0, cache_read: 0.3, cache_write: 3.75 },
                            haiku: { input: 0.25, output: 1.25, cache_read: 0.03, cache_write: 0.3 },
                        },
                    },
                    ui: { theme: 'auto', default_page: 'dashboard', sessions_per_page: 50 },
                };
                const result = await API.saveConfig(defaultCfg);
                this.config = result;
                document.documentElement.setAttribute('data-theme', 'auto');
                this.renderContent();
                App.toast('Settings reset to defaults', 'success');
            } catch (err) {
                App.toast('Failed to reset settings: ' + err.message, 'error');
            }
        });
    },

    bindAddModelButton() {
        const btn = document.getElementById('add-model-btn');
        if (!btn) return;

        btn.addEventListener('click', () => {
            const name = prompt('Enter model name (e.g., "opus-4"):');
            if (!name || !name.trim()) return;
            const key = name.trim().toLowerCase();
            if (!this.config.cost) this.config.cost = {};
            if (!this.config.cost.models) this.config.cost.models = {};
            if (this.config.cost.models[key]) {
                App.toast('Model "' + key + '" already exists', 'warning');
                return;
            }
            this.config.cost.models[key] = { input: 0, output: 0, cache_read: 0, cache_write: 0 };
            this.renderModelPricingTable();
        });
    },

    /** Collect all form values back into this.config */
    collectFormValues() {
        // Toggle switches and inputs with data-path
        document.querySelectorAll('[data-path]').forEach(el => {
            const path = el.dataset.path;
            const parts = path.split('.');
            let val;

            if (el.type === 'checkbox') {
                val = el.checked;
            } else if (el.type === 'number') {
                val = parseFloat(el.value) || 0;
            } else {
                val = el.value;
            }

            // Set nested value
            let obj = this.config;
            for (let i = 0; i < parts.length - 1; i++) {
                if (!obj[parts[i]]) obj[parts[i]] = {};
                obj = obj[parts[i]];
            }
            obj[parts[parts.length - 1]] = val;
        });

        // Model pricing
        document.querySelectorAll('tr[data-model]').forEach(row => {
            const modelName = row.dataset.model;
            if (!this.config.cost) this.config.cost = {};
            if (!this.config.cost.models) this.config.cost.models = {};
            if (!this.config.cost.models[modelName]) this.config.cost.models[modelName] = {};

            row.querySelectorAll('[data-model-field]').forEach(input => {
                const field = input.dataset.modelField;
                this.config.cost.models[modelName][field] = parseFloat(input.value) || 0;
            });
        });
    },

    /* ------------------------------------------------------------------
       YAML Tab
    ------------------------------------------------------------------ */
    renderYamlTab(area) {
        // Convert config to a YAML-like display
        const yamlText = this.configToYaml(this.config);

        area.innerHTML = `
            <div class="card mt-2">
                <div class="card-header">
                    <h3>Configuration (YAML)</h3>
                    <div class="flex gap-1">
                        <button class="btn btn-primary btn-sm" id="save-yaml-btn">Save YAML</button>
                    </div>
                </div>
                <textarea id="yaml-editor" class="input" style="width:100%;min-height:500px;font-family:var(--font-mono);font-size:0.85rem;resize:vertical;background:var(--bg-code);color:var(--text-code);padding:16px;border-radius:var(--radius-md);">${this._esc(yamlText)}</textarea>
            </div>
        `;

        document.getElementById('save-yaml-btn').addEventListener('click', async () => {
            const textarea = document.getElementById('yaml-editor');
            if (!textarea) return;

            try {
                // Parse the YAML text as JSON (since we output JSON-compatible YAML)
                // In practice the server expects JSON, so we parse the YAML-like text
                const parsed = this.parseSimpleYaml(textarea.value);
                const result = await API.saveConfig(parsed);
                this.config = result;
                App.toast('Configuration saved successfully', 'success');
            } catch (err) {
                App.toast('Failed to save: ' + err.message, 'error');
            }
        });
    },

    /** Convert config object to a simple YAML-like string */
    configToYaml(obj, indent) {
        indent = indent || 0;
        const pad = '  '.repeat(indent);
        let result = '';

        for (const [key, val] of Object.entries(obj)) {
            if (val === null || val === undefined) {
                result += pad + key + ': null\n';
            } else if (typeof val === 'object' && !Array.isArray(val)) {
                result += pad + key + ':\n';
                result += this.configToYaml(val, indent + 1);
            } else if (typeof val === 'boolean') {
                result += pad + key + ': ' + (val ? 'true' : 'false') + '\n';
            } else if (typeof val === 'number') {
                result += pad + key + ': ' + val + '\n';
            } else {
                result += pad + key + ': "' + String(val).replace(/"/g, '\\"') + '"\n';
            }
        }
        return result;
    },

    /** Parse simple YAML-like text back to a config object */
    parseSimpleYaml(text) {
        // Simple YAML parser for our known config structure
        // Handles key: value, key:\n (object start), booleans, numbers, strings
        const lines = text.split('\n');
        const root = {};
        const stack = [{ obj: root, indent: -1 }];

        for (const line of lines) {
            if (line.trim() === '' || line.trim().startsWith('#')) continue;

            const match = line.match(/^(\s*)([\w_]+)\s*:\s*(.*)/);
            if (!match) continue;

            const indent = match[1].length;
            const key = match[2];
            let rawVal = match[3].trim();

            // Pop stack to find correct parent
            while (stack.length > 1 && stack[stack.length - 1].indent >= indent) {
                stack.pop();
            }

            const parent = stack[stack.length - 1].obj;

            if (rawVal === '' || rawVal === null) {
                // Object start
                parent[key] = {};
                stack.push({ obj: parent[key], indent: indent });
            } else {
                // Parse value
                let val;
                if (rawVal === 'true') val = true;
                else if (rawVal === 'false') val = false;
                else if (rawVal === 'null') val = null;
                else if (/^-?\d+(\.\d+)?$/.test(rawVal)) val = parseFloat(rawVal);
                else {
                    // Remove surrounding quotes
                    val = rawVal.replace(/^["']|["']$/g, '');
                }
                parent[key] = val;
            }
        }

        return root;
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

    destroy() {
        // No charts to destroy, but clean up any state
    },
};
