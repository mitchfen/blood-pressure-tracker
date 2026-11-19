let chart = null;
let allBPs = [];
let isShowingAllHistory = false;

const MOBILE_BREAKPOINT = 600;
const HISTORY_LIMIT = 30;

function isMobileViewport() {
    return window.innerWidth <= MOBILE_BREAKPOINT;
}

function getDisplayBPs() {
    if (isShowingAllHistory || allBPs.length <= HISTORY_LIMIT) {
        return allBPs;
    }
    return allBPs.slice(-HISTORY_LIMIT);
}

function updateHistoryToggle() {
    const toggle = document.getElementById('history-toggle');
    if (!toggle) return;

    if (allBPs.length > HISTORY_LIMIT) {
        toggle.style.display = 'inline-block';
        toggle.textContent = isShowingAllHistory
            ? 'Show Recent 30 Days'
            : 'Show Full History';
    } else {
        toggle.style.display = 'none';
    }
}

function toggleHistory() {
    isShowingAllHistory = !isShowingAllHistory;
    renderChart();
}

function getHealthZone(systolic, diastolic) {
    if (systolic > 180 || diastolic > 120) {
        return { name: 'Hypertensive Crisis', color: '#e11d48' };
    }
    if (systolic >= 140 || diastolic >= 90) {
        return { name: 'Stage 2', color: '#e11d48' };
    }
    if ((systolic >= 130 && systolic <= 139) || (diastolic >= 80 && diastolic <= 89)) {
        return { name: 'Stage 1', color: '#ea580c' };
    }
    if ((systolic >= 120 && systolic <= 129) && diastolic < 80) {
        return { name: 'Elevated', color: '#d97706' };
    }
    return { name: 'Normal', color: '#059669' };
}

function renderChart() {
    const bps = getDisplayBPs();
    const emptyState = document.getElementById('chart-empty-state');
    const canvas = document.getElementById('bpChart');
    const toggle = document.getElementById('history-toggle');

    if (!bps || bps.length === 0) {
        if (chart) {
            chart.destroy();
            chart = null;
        }
        if (emptyState) emptyState.style.display = 'block';
        if (canvas) canvas.style.display = 'none';
        if (toggle) toggle.style.display = 'none';
        return;
    }

    if (emptyState) emptyState.style.display = 'none';
    if (canvas) canvas.style.display = 'block';

    const labels = bps.map(w => new Date(w.recorded_at).toLocaleDateString());
    const systolicData = bps.map(w => w.systolic);
    const diastolicData = bps.map(w => w.diastolic);

    const ctx = document.getElementById('bpChart').getContext('2d');

    if (chart) {
        chart.destroy();
    }

    const mobileViewport = isMobileViewport();

    chart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                {
                    label: 'Systolic',
                    data: systolicData,
                    borderColor: '#f43f5e', // Vibrant rose matching gamification theme
                    backgroundColor: 'rgba(244, 63, 94, 0.12)',
                    borderWidth: 3,
                    fill: false,
                    tension: 0.3,
                    pointRadius: mobileViewport ? 3 : 5,
                    pointBackgroundColor: '#f43f5e',
                    pointBorderColor: '#090d16',
                    pointBorderWidth: mobileViewport ? 1.5 : 2,
                    pointHoverRadius: mobileViewport ? 5 : 7,
                    zIndex: 2
                },
                {
                    label: 'Diastolic',
                    data: diastolicData,
                    borderColor: '#38bdf8', // Vibrant sky blue matching ambient glow
                    backgroundColor: 'rgba(244, 63, 94, 0.08)', // Fill area between curves
                    borderWidth: 3,
                    fill: '-1', // Fill to systolic
                    tension: 0.3,
                    pointRadius: mobileViewport ? 3 : 5,
                    pointBackgroundColor: '#38bdf8',
                    pointBorderColor: '#090d16',
                    pointBorderWidth: mobileViewport ? 1.5 : 2,
                    pointHoverRadius: mobileViewport ? 5 : 7,
                    zIndex: 1
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            interaction: {
                mode: 'index',
                intersect: false,
            },
            plugins: {
                legend: {
                    display: true,
                    position: 'top',
                    labels: {
                        boxWidth: mobileViewport ? 12 : 32,
                        usePointStyle: true,
                        color: '#cbd5e1',
                        font: { family: 'Inter', weight: '600' }
                    }
                },
                tooltip: {
                    backgroundColor: 'rgba(15, 23, 42, 0.95)',
                    titleColor: '#ffffff',
                    bodyColor: '#cbd5e1',
                    borderColor: 'rgba(51, 65, 85, 0.8)',
                    borderWidth: 1,
                    padding: 12,
                    cornerRadius: 8,
                    callbacks: {
                        label: function(context) {
                            return context.dataset.label + ': ' + context.parsed.y + ' mmHg';
                        },
                        afterBody: function(contexts) {
                            if (contexts.length > 0) {
                                const index = contexts[0].dataIndex;
                                const bp = bps[index];
                                if (bp) {
                                    const zone = getHealthZone(bp.systolic, bp.diastolic);
                                    const pulseGap = bp.systolic - bp.diastolic;
                                    return [
                                        'AHA Zone: ' + zone.name,
                                        'Pulse Pressure: ' + pulseGap + ' mmHg'
                                    ];
                                }
                            }
                            return [];
                        }
                    }
                }
            },
            scales: {
                x: {
                    ticks: {
                        color: '#94a3b8',
                        maxTicksLimit: mobileViewport ? 5 : 12,
                        maxRotation: mobileViewport ? 0 : 45
                    },
                    grid: { color: 'rgba(255, 255, 255, 0.06)' }
                },
                y: {
                    min: Math.floor(Math.min(...diastolicData) / 10) * 10 - 10,
                    max: Math.ceil(Math.max(...systolicData) / 10) * 10 + 10,
                    ticks: {
                        color: '#94a3b8',
                        stepSize: 10,
                        maxTicksLimit: mobileViewport ? 6 : 10
                    },
                    grid: { color: 'rgba(255, 255, 255, 0.06)' }
                }
            }
        }
    });

    updateHistoryToggle();
}

async function loadChart() {
    try {
        const response = await fetch('/api/bps');
        allBPs = await response.json();
        renderChart();
    } catch (error) {
        console.error('Failed to load chart:', error);
    }
}

async function loadStats() {
    try {
        const res = await fetch('/api/stats');
        if (!res.ok) return;
        const stats = await res.json();

        // Update Level & XP
        const userLevel = document.getElementById('user-level');
        const levelTitle = document.getElementById('level-title');
        const totalXp = document.getElementById('user-total-xp');
        const nextLevel = document.getElementById('next-level');
        const xpInLevel = document.getElementById('xp-in-level');
        const xpNeeded = document.getElementById('xp-needed');
        const progressPercent = document.getElementById('progress-percent');
        const xpBar = document.getElementById('xp-bar');

        if (userLevel) userLevel.textContent = stats.level;
        if (levelTitle) levelTitle.textContent = stats.level_title || 'Heart Guardian';
        if (totalXp) totalXp.textContent = (stats.xp || 0).toLocaleString();
        if (nextLevel) nextLevel.textContent = stats.level + 1;
        if (xpInLevel) xpInLevel.textContent = stats.xp_in_level || 0;
        if (xpNeeded) xpNeeded.textContent = stats.xp_needed || 100;
        if (progressPercent) progressPercent.textContent = stats.progress_percent || 0;
        if (xpBar) xpBar.style.width = Math.min(100, Math.max(0, stats.progress_percent || 0)) + '%';

        // Update Streak
        const streakCount = document.getElementById('streak-count');
        const longestStreak = document.getElementById('longest-streak');
        const flame = document.getElementById('streak-flame');

        if (streakCount) streakCount.textContent = stats.current_streak || 0;
        if (longestStreak) longestStreak.textContent = stats.longest_streak || 0;
        if (flame) {
            if (stats.current_streak > 0) {
                flame.classList.remove('opacity-40', 'grayscale');
                flame.classList.add('animate-pulse');
            } else {
                flame.classList.add('opacity-40', 'grayscale');
                flame.classList.remove('animate-pulse');
            }
        }

        // Update Latest Reading Card
        const latestBP = document.getElementById('latest-bp-reading');
        const latestZone = document.getElementById('latest-zone-badge');
        const pulsePressureVal = document.getElementById('pulse-pressure-val');

        if (stats.latest_reading) {
            if (latestBP) latestBP.textContent = `${stats.latest_reading.systolic}/${stats.latest_reading.diastolic} mmHg`;
            if (pulsePressureVal) pulsePressureVal.textContent = stats.latest_reading.pulse_pressure;
            if (latestZone && stats.latest_reading.zone) {
                const z = stats.latest_reading.zone;
                latestZone.className = `px-2.5 py-0.5 rounded-md font-semibold text-xs border ${z.bg_class} ${z.text_class} ${z.border_class}`;
                latestZone.textContent = z.name;
            }
        } else {
            if (latestBP) latestBP.textContent = '--/--';
            if (pulsePressureVal) pulsePressureVal.textContent = '--';
            if (latestZone) {
                latestZone.className = 'px-2.5 py-0.5 rounded-md font-semibold text-xs bg-slate-700 text-slate-300 border border-slate-600';
                latestZone.textContent = 'No data yet';
            }
        }
    } catch (err) {
        console.error('Failed to load stats:', err);
    }
}

function triggerCelebration(detail) {
    if (typeof confetti === 'function') {
        if (detail && detail.leveledUp) {
            // Level-up celebration: prolonged fireworks
            const duration = 2.5 * 1000;
            const animationEnd = Date.now() + duration;
            const defaults = { startVelocity: 30, spread: 360, ticks: 60, zIndex: 1000 };

            function randomInRange(min, max) {
                return Math.random() * (max - min) + min;
            }

            const interval = setInterval(function() {
                const timeLeft = animationEnd - Date.now();
                if (timeLeft <= 0) {
                    return clearInterval(interval);
                }
                const particleCount = 40 * (timeLeft / duration);
                confetti(Object.assign({}, defaults, { particleCount, origin: { x: randomInRange(0.1, 0.4), y: Math.random() - 0.2 } }));
                confetti(Object.assign({}, defaults, { particleCount, origin: { x: randomInRange(0.6, 0.9), y: Math.random() - 0.2 } }));
            }, 250);
        } else {
            // Standard reading celebration
            confetti({
                particleCount: 65,
                spread: 70,
                origin: { y: 0.65 },
                colors: ['#dc2626', '#f59e0b', '#10b981', '#3b82f6', '#8b5cf6']
            });
        }
    }
}

window.addEventListener('resize', renderChart);

// Load chart and stats on initial page load
document.addEventListener('DOMContentLoaded', () => {
    loadChart();
    loadStats();
});

// React to HTMX bpUpdated event
document.body.addEventListener('bpUpdated', function(evt) {
    let detail = evt.detail || {};
    // If detail is wrapped or parsed from JSON
    if (detail.value && typeof detail.value === 'object') {
        detail = detail.value;
    }

    triggerCelebration(detail);
    loadChart();
    loadStats();

    // Clear success message after 4.5 seconds
    setTimeout(() => {
        const msg = document.getElementById('message-container');
        if (msg) {
            msg.style.transition = 'opacity 0.4s ease';
            msg.style.opacity = '0';
            setTimeout(() => {
                msg.innerHTML = '';
                msg.style.opacity = '1';
            }, 400);
        }
    }, 4500);
});
