/* ==========================================================================
   Claude Monitor — API Client
   ========================================================================== */

const API = {
    /**
     * Perform a GET request against the API.
     * @param {string} endpoint - The API path (e.g. "/api/sessions").
     * @param {Object} params  - Query parameters (falsy values are omitted).
     * @returns {Promise<any>}
     */
    async get(endpoint, params = {}) {
        const url = new URL(endpoint, window.location.origin);
        Object.entries(params).forEach(([k, v]) => {
            if (v !== undefined && v !== null && v !== '') {
                url.searchParams.set(k, v);
            }
        });
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`API error: ${response.status} ${response.statusText}`);
        }
        return response.json();
    },

    /**
     * Perform a POST request against the API.
     * @param {string} endpoint - The API path.
     * @param {Object} body     - JSON body.
     * @returns {Promise<any>}
     */
    async post(endpoint, body) {
        const response = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!response.ok) {
            throw new Error(`API error: ${response.status} ${response.statusText}`);
        }
        return response.json();
    },

    // ---- Convenience methods ----

    /** List sessions with optional filters. */
    getSessions(params) {
        return API.get('/api/sessions', params);
    },

    /** Get a single session by ID. */
    getSession(id) {
        return API.get(`/api/sessions/${id}`);
    },

    /** Get aggregate stats. */
    getStats() {
        return API.get('/api/stats');
    },

    /** Get daily stats for the last N days. */
    getDailyStats(days) {
        return API.get('/api/stats/daily', { days });
    },

    /** Get tool usage stats. */
    getToolStats() {
        return API.get('/api/stats/tools');
    },

    /** Get model usage stats. */
    getModelStats() {
        return API.get('/api/stats/models');
    },

    /** Get project stats. */
    getProjectStats() {
        return API.get('/api/stats/projects');
    },

    /** Get current config. */
    getConfig() {
        return API.get('/api/config');
    },

    /** Save config. */
    saveConfig(cfg) {
        return API.post('/api/config', cfg);
    },

    /** Full-text search. */
    search(q, limit) {
        return API.get('/api/search', { q, limit });
    },
};
