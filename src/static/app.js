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
                    borderColor: '#dc2626', // Tailwind red-600
                    backgroundColor: 'rgba(220, 38, 38, 0.1)',
                    borderWidth: 3,
                    fill: false, // Don't fill down to zero
                    tension: 0.3,
                    pointRadius: mobileViewport ? 2 : 4,
                    pointBackgroundColor: '#dc2626',
                    pointBorderColor: '#fff',
                    pointBorderWidth: mobileViewport ? 1 : 2,
                    pointHoverRadius: mobileViewport ? 4 : 7,
                    zIndex: 2
                },
                {
                    label: 'Diastolic',
                    data: diastolicData,
                    borderColor: '#2563eb', // Tailwind blue-600
                    backgroundColor: 'rgba(220, 38, 38, 0.1)', // Fill area color
                    borderWidth: 3,
                    fill: '-1', // Fill space to the previous dataset (Systolic)
                    tension: 0.3,
                    pointRadius: mobileViewport ? 2 : 4,
                    pointBackgroundColor: '#2563eb',
                    pointBorderColor: '#fff',
                    pointBorderWidth: mobileViewport ? 1 : 2,
                    pointHoverRadius: mobileViewport ? 4 : 7,
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
                        boxWidth: mobileViewport ? 10 : 40,
                        usePointStyle: mobileViewport
                    }
                },
                tooltip: {
                    callbacks: {
                        label: function(context) {
                            return context.dataset.label + ': ' + context.parsed.y;
                        }
                    }
                }
            },
            scales: {
                x: {
                    ticks: {
                        maxTicksLimit: mobileViewport ? 5 : 12,
                        maxRotation: mobileViewport ? 0 : 45
                    }
                },
                y: {
                    min: Math.floor(Math.min(...diastolicData) / 10) * 10 - 10,
                    max: Math.ceil(Math.max(...systolicData) / 10) * 10 + 10,
                    ticks: {
                        stepSize: 10,
                        maxTicksLimit: mobileViewport ? 6 : 10
                    }
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

window.addEventListener('resize', renderChart);

// Load chart on page load
document.addEventListener('DOMContentLoaded', loadChart);

// Reload chart on form submission
document.addEventListener('DOMContentLoaded', function() {
    document.querySelector('form').addEventListener('htmx:afterRequest', function(event) {
        const messageContainer = document.getElementById('message-container');
        
        if (event.detail.xhr.status === 200) {
            messageContainer.innerHTML = '<div class="message success">✓ Blood pressure recorded successfully!</div>';
            document.getElementById('systolic').value = '';
            document.getElementById('diastolic').value = '';
            setTimeout(() => {
                messageContainer.innerHTML = '';
                loadChart();
            }, 1500);
        } else {
            messageContainer.innerHTML = '<div class="message error">✗ Failed to record blood pressure</div>';
            setTimeout(() => {
                messageContainer.innerHTML = '';
            }, 3000);
        }
    });
});

async function importCSV(input) {
    const messageContainer = document.getElementById('message-container');
    const formData = new FormData();
    formData.append('file', input.files[0]);

    try {
        const response = await fetch('/api/bps/import', { method: 'POST', body: formData });
        const result = await response.json();
        messageContainer.innerHTML = `<div class="message success">✓ Imported ${result.imported} records</div>`;
        input.value = '';
        setTimeout(() => { messageContainer.innerHTML = ''; loadChart(); }, 1500);
    } catch (error) {
        messageContainer.innerHTML = '<div class="message error">✗ Failed to import CSV</div>';
        setTimeout(() => { messageContainer.innerHTML = ''; }, 3000);
    }
}
