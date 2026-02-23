document.addEventListener('DOMContentLoaded', () => {
    // Navigation Logic
    const navBtns = document.querySelectorAll('.nav-btn');
    const sections = document.querySelectorAll('.view-section');

    navBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            // Update buttons
            navBtns.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            
            // Update sections
            const targetId = btn.getAttribute('data-target');
            sections.forEach(sec => {
                if (sec.id === targetId) {
                    sec.classList.add('active');
                    if (targetId === 'cron-view') loadCronJobs();
                    if (targetId === 'settings-view' || targetId === 'channels-view') loadConfig();
                } else {
                    sec.classList.remove('active');
                }
            });
        });
    });

    // Chat Logic
    const chatForm = document.getElementById('chat-form');
    const chatInput = document.getElementById('chat-input');
    const chatMessages = document.getElementById('chat-messages');

    let currentSessionId = 'sess-' + Math.random().toString(36).substring(2, 9);
    
    // Setup SSE for incoming stream
    let eventSource = null;

    function connectSSE() {
        if (eventSource) eventSource.close();
        
        eventSource = new EventSource('/api/chat/stream?session=' + currentSessionId);
        
        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                appendMessage('assistant', data.content);
            } catch(e) {
                console.error("Failed to parse SSE message", e);
            }
        };

        eventSource.onerror = (err) => {
            console.error("SSE Error:", err);
            eventSource.close();
            setTimeout(connectSSE, 5000); // Try reconnecting
        };
    }

    connectSSE();

    function appendMessage(role, content) {
        const msgDiv = document.createElement('div');
        msgDiv.className = `message ${role}`;
        
        const avatar = document.createElement('div');
        avatar.className = 'avatar';
        avatar.textContent = role === 'user' ? '👤' : '🤖';
        
        const bubble = document.createElement('div');
        bubble.className = 'bubble';
        bubble.textContent = content;
        
        msgDiv.appendChild(avatar);
        msgDiv.appendChild(bubble);
        chatMessages.appendChild(msgDiv);
        
        chatMessages.scrollTop = chatMessages.scrollHeight;
    }

    chatForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        const text = chatInput.value.trim();
        if (!text) return;

        appendMessage('user', text);
        chatInput.value = '';

        try {
            await fetch('/api/chat', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ text, sessionId: currentSessionId })
            });
        } catch (err) {
            console.error("Failed to send message", err);
            appendMessage('tool', 'Error: Failed to send message to server.');
        }
    });

    // -------------------------------------------------------------
    // Config & Channels form handling
    // We use a flat notation in names like `channels.telegram.enabled`
    // but the backend will just respond/accept the raw JSON structure
    // -------------------------------------------------------------
    
    let currentConfigData = null;

    async function loadConfig() {
        try {
            const res = await fetch('/api/config');
            if (res.ok) {
                currentConfigData = await res.json();
                populateForm(document.getElementById('config-form'), currentConfigData);
                populateForm(document.getElementById('channels-form'), currentConfigData);
            }
        } catch (e) {
            console.error("Failed to load config", e);
        }
    }

    function populateForm(form, config) {
        // Simple mapping based on known paths
        const map = {
            'agents.defaults.model': config?.agents?.defaults?.model || '',
            'agents.defaults.workspace': config?.agents?.defaults?.workspace || '',
            'agents.defaults.temperature': config?.agents?.defaults?.temperature || 0.7,
            'agents.defaults.maxtokens': config?.agents?.defaults?.maxTokens || 8192,
            
            'providers.openai.apikey': config?.providers?.openai?.apiKey || '',
            'providers.openai.apibase': config?.providers?.openai?.apiBase || '',
            
            'channels.telegram.enabled': config?.channels?.telegram?.enabled || false,
            'channels.telegram.token': config?.channels?.telegram?.token || '',
            'channels.telegram.allowfrom': (config?.channels?.telegram?.allowFrom || []).join(', '),
            
            'channels.discord.enabled': config?.channels?.discord?.enabled || false,
            'channels.discord.token': config?.channels?.discord?.token || '',
            'channels.discord.allowfrom': (config?.channels?.discord?.allowFrom || []).join(', ')
        };

        for (const [name, val] of Object.entries(map)) {
            const input = form.querySelector(`[name="${name}"]`);
            if (input) {
                if (input.type === 'checkbox') {
                    input.checked = val;
                } else {
                    input.value = val;
                }
            }
        }
    }

    function formToConfigConfig(form) {
        // Deep clone current config
        let cfg = JSON.parse(JSON.stringify(currentConfigData || {}));
        
        // Ensure structure exists
        if(!cfg.agents) cfg.agents = {defaults: {}};
        if(!cfg.providers) cfg.providers = {openai: {}};
        if(!cfg.channels) cfg.channels = {telegram: {}, discord: {}};

        // Parse form elements
        const inputs = form.querySelectorAll('input');
        inputs.forEach(el => {
            const name = el.getAttribute('name');
            if (!name) return;
            
            const val = el.type === 'checkbox' ? el.checked : el.value;
            
            // Basic path matching based on name attr
            switch(name) {
                case 'agents.defaults.model': cfg.agents.defaults.model = val; break;
                case 'agents.defaults.workspace': cfg.agents.defaults.workspace = val; break;
                case 'agents.defaults.temperature': cfg.agents.defaults.temperature = parseFloat(val); break;
                case 'agents.defaults.maxtokens': cfg.agents.defaults.maxTokens = parseInt(val, 10); break;
                
                case 'providers.openai.apikey': cfg.providers.openai.apiKey = val; break;
                case 'providers.openai.apibase': cfg.providers.openai.apiBase = val; break;
                
                case 'channels.telegram.enabled': cfg.channels.telegram.enabled = val; break;
                case 'channels.telegram.token': cfg.channels.telegram.token = val; break;
                case 'channels.telegram.allowfrom': 
                    cfg.channels.telegram.allowFrom = val ? val.split(',').map(s=>s.trim()).filter(s=>s) : []; 
                    break;
                
                case 'channels.discord.enabled': cfg.channels.discord.enabled = val; break;
                case 'channels.discord.token': cfg.channels.discord.token = val; break;
                case 'channels.discord.allowfrom': 
                    cfg.channels.discord.allowFrom = val ? val.split(',').map(s=>s.trim()).filter(s=>s) : []; 
                    break;
            }
        });
        return cfg;
    }

    async function handleConfigSubmit(form, statusEl) {
        const newCfg = formToConfigConfig(form);
        statusEl.textContent = 'Saving...';
        statusEl.className = 'status-msg';
        
        try {
            const res = await fetch('/api/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(newCfg)
            });
            
            if (res.ok) {
                statusEl.textContent = 'Saved successfully!';
                statusEl.classList.add('success');
                currentConfigData = newCfg;
            } else {
                throw new Error("HTTP " + res.status);
            }
        } catch (e) {
            statusEl.textContent = 'Error saving config.';
            statusEl.classList.add('error');
            console.error(e);
        }
        
        setTimeout(() => statusEl.textContent = '', 3000);
    }

    document.getElementById('config-form').addEventListener('submit', (e) => {
        e.preventDefault();
        handleConfigSubmit(e.target, document.getElementById('config-status'));
    });

    document.getElementById('channels-form').addEventListener('submit', (e) => {
        e.preventDefault();
        handleConfigSubmit(e.target, document.getElementById('channels-status'));
    });

    // -------------------------------------------------------------
    // Cron Jobs
    // -------------------------------------------------------------

    const cronList = document.getElementById('cron-list');
    document.getElementById('refresh-cron').addEventListener('click', loadCronJobs);

    async function loadCronJobs() {
        cronList.innerHTML = '<div class="empty-state">Loading cron jobs...</div>';
        try {
            const res = await fetch('/api/cron');
            if (res.ok) {
                const jobs = await res.json();
                renderCronJobs(jobs);
            }
        } catch(e) {
            cronList.innerHTML = `<div class="empty-state text-error">Failed to load cron jobs</div>`;
            console.error(e);
        }
    }

    function renderCronJobs(jobs) {
        if (!jobs || jobs.length === 0) {
            cronList.innerHTML = '<div class="empty-state">No scheduled tasks found.</div>';
            return;
        }

        cronList.innerHTML = '';
        jobs.forEach(job => {
            const item = document.createElement('div');
            item.className = 'list-item';
            
            // Compute visual firing time
            const fireDate = new Date(job.FireAt);
            const isRecurring = job.Recurring ? `Recurring (${job.Interval/1000/60000000}m)` : 'One-time';

            item.innerHTML = `
                <div class="item-info">
                    <h5>${job.Name}</h5>
                    <p>${job.Message.substring(0, 50)}${job.Message.length > 50 ? '...' : ''}</p>
                    <span class="item-meta">${isRecurring} | Fires: ${fireDate.toLocaleString()}</span>
                </div>
                <div class="item-actions">
                    <button class="btn btn-danger delete-cron" data-id="${job.ID}">Cancel</button>
                </div>
            `;
            cronList.appendChild(item);
        });

        // Add delete handlers
        document.querySelectorAll('.delete-cron').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const id = e.target.getAttribute('data-id');
                try {
                    const r = await fetch(`/api/cron/${id}`, { method: 'DELETE' });
                    if (r.ok) {
                        e.target.closest('.list-item').remove();
                        if (cronList.children.length === 0) {
                            cronList.innerHTML = '<div class="empty-state">No scheduled tasks found.</div>';
                        }
                    } else {
                        alert("Failed to delete cron job");
                    }
                } catch(error) {
                    console.error(error);
                }
            });
        });
    }

    // Initial load
    loadConfig();
});
