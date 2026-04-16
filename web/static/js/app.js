/* ==========================================================================
   Claude Monitor — SPA Router & State Management
   ========================================================================== */

const App = {
    currentPage: 'dashboard',
    filters: {
        project: '',
        from: '',
        to: '',
        model: '',
        search: '',
    },

    /** Page registry — maps page names to component objects. */
    pages: {
        dashboard: typeof DashboardPage !== 'undefined' ? DashboardPage : null,
        sessions:  typeof SessionsPage  !== 'undefined' ? SessionsPage  : null,
        live:      typeof LivePage      !== 'undefined' ? LivePage      : null,
        tools:     typeof ToolsPage     !== 'undefined' ? ToolsPage     : null,
        agents:    typeof AgentsPage    !== 'undefined' ? AgentsPage    : null,
        projects:  typeof ProjectsPage  !== 'undefined' ? ProjectsPage  : null,
        cost:      typeof CostPage      !== 'undefined' ? CostPage      : null,
        settings:  typeof SettingsPage  !== 'undefined' ? SettingsPage  : null,
    },

    /* ------------------------------------------------------------------
       Initialisation
    ------------------------------------------------------------------ */
    init() {
        this.setupRouter();
        this.setupFilters();
        this.setupSidebarToggle();
        this.navigate(window.location.hash.slice(1) || 'dashboard');
        this.populateFilterDropdowns();
    },

    /* ------------------------------------------------------------------
       Router
    ------------------------------------------------------------------ */
    setupRouter() {
        window.addEventListener('hashchange', () => {
            this.navigate(window.location.hash.slice(1));
        });

        document.querySelectorAll('.nav-link[data-page]').forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const page = link.dataset.page;
                window.location.hash = page;
            });
        });
    },

    navigate(page) {
        if (!page || !this.pages.hasOwnProperty(page)) {
            page = 'dashboard';
        }

        this.currentPage = page;

        // Update active sidebar link
        document.querySelectorAll('.nav-link').forEach(link => {
            link.classList.toggle('active', link.dataset.page === page);
        });

        // Close mobile sidebar if open
        document.getElementById('sidebar').classList.remove('open');
        document.getElementById('sidebar-toggle').classList.remove('open');
        const overlay = document.querySelector('.sidebar-overlay');
        if (overlay) overlay.classList.remove('active');

        // Render page component
        const container = document.getElementById('main-content');
        container.innerHTML = '';

        const component = this.pages[page];
        if (component && typeof component.render === 'function') {
            try {
                component.render(container);
            } catch (err) {
                container.innerHTML = `<div class="empty-state"><h3>Error</h3><p>${err.message}</p></div>`;
                console.error('Page render error:', err);
            }
        } else {
            container.innerHTML = '<div class="empty-state"><h3>Page not found</h3></div>';
        }

        // Update URL hash without triggering hashchange again
        if (window.location.hash.slice(1) !== page) {
            history.replaceState(null, '', '#' + page);
        }
    },

    /* ------------------------------------------------------------------
       Filters
    ------------------------------------------------------------------ */
    setupFilters() {
        const ids = {
            search:  'filter-search',
            from:    'filter-from',
            to:      'filter-to',
            project: 'filter-project',
            model:   'filter-model',
        };

        Object.entries(ids).forEach(([key, id]) => {
            const el = document.getElementById(id);
            if (!el) return;
            el.addEventListener('change', () => {
                this.filters[key] = el.value;
                this.onFiltersChanged();
            });
            // Also listen to input for search field (instant typing)
            if (key === 'search') {
                let debounce;
                el.addEventListener('input', () => {
                    clearTimeout(debounce);
                    debounce = setTimeout(() => {
                        this.filters.search = el.value;
                        this.onFiltersChanged();
                    }, 300);
                });
            }
        });
    },

    onFiltersChanged() {
        // Re-render current page with updated filters
        const container = document.getElementById('main-content');
        const component = this.pages[this.currentPage];
        if (component && typeof component.render === 'function') {
            container.innerHTML = '';
            try {
                component.render(container);
            } catch (err) {
                console.error('Filter re-render error:', err);
            }
        }
    },

    /** Build a query-string-ready params object from current filters. */
    getFilterParams() {
        const p = {};
        if (this.filters.project) p.project = this.filters.project;
        if (this.filters.from)    p.from    = this.filters.from;
        if (this.filters.to)      p.to      = this.filters.to;
        if (this.filters.model)   p.model   = this.filters.model;
        if (this.filters.search)  p.search  = this.filters.search;
        return p;
    },

    /** Populate project and model dropdowns from the API. */
    async populateFilterDropdowns() {
        try {
            const projectSelect = document.getElementById('filter-project');
            const modelSelect   = document.getElementById('filter-model');

            const [projects, models] = await Promise.allSettled([
                API.getProjectStats(),
                API.getModelStats(),
            ]);

            if (projects.status === 'fulfilled' && Array.isArray(projects.value)) {
                projects.value.forEach(p => {
                    const name = p.project || p.name || p;
                    if (!name) return;
                    const opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = name;
                    projectSelect.appendChild(opt);
                });
            }

            if (models.status === 'fulfilled' && Array.isArray(models.value)) {
                models.value.forEach(m => {
                    const name = m.model || m.name || m;
                    if (!name) return;
                    const opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = name;
                    modelSelect.appendChild(opt);
                });
            }
        } catch (err) {
            // Silently ignore — dropdowns just stay with "All" option
            console.warn('Could not populate filter dropdowns:', err);
        }
    },

    /* ------------------------------------------------------------------
       Mobile Sidebar
    ------------------------------------------------------------------ */
    setupSidebarToggle() {
        const toggle  = document.getElementById('sidebar-toggle');
        const sidebar = document.getElementById('sidebar');

        // Create overlay
        const overlay = document.createElement('div');
        overlay.className = 'sidebar-overlay';
        document.body.appendChild(overlay);

        toggle.addEventListener('click', () => {
            const isOpen = sidebar.classList.toggle('open');
            toggle.classList.toggle('open', isOpen);
            overlay.classList.toggle('active', isOpen);
        });

        overlay.addEventListener('click', () => {
            sidebar.classList.remove('open');
            toggle.classList.remove('open');
            overlay.classList.remove('active');
        });
    },

    /* ------------------------------------------------------------------
       Toast Helper
    ------------------------------------------------------------------ */
    /**
     * Show a toast notification.
     * @param {string} message - Text to display.
     * @param {'success'|'error'|'warning'|'info'} type - Toast variant.
     * @param {number} duration - Auto-dismiss time in ms (default 4000).
     */
    toast(message, type = 'info', duration = 4000) {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.textContent = message;
        container.appendChild(toast);

        setTimeout(() => {
            toast.classList.add('toast-exit');
            toast.addEventListener('animationend', () => toast.remove());
        }, duration);
    },
};

/* Boot */
document.addEventListener('DOMContentLoaded', () => App.init());
