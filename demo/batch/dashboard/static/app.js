let pollInterval = null;
let startTime = null;
let timerInterval = null;

function startScenario(name) {
  const btn = document.getElementById('btn-run');
  btn.disabled = true;
  btn.textContent = 'Starting...';
  startTime = Date.now();

  fetch(`/api/run/${name}`, { method: 'POST' })
    .then(r => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      return r.json();
    })
    .then(() => {
      startPolling(name);
      startTimer();
    })
    .catch(err => {
      btn.disabled = false;
      btn.textContent = 'Run';
      alert('Failed to start: ' + err.message);
    });
}

function startPolling(name) {
  pollStatus(name);
  pollInterval = setInterval(() => pollStatus(name), 2000);
}

function stopPolling() {
  if (pollInterval) {
    clearInterval(pollInterval);
    pollInterval = null;
  }
}

function startTimer() {
  updateTimer();
  timerInterval = setInterval(updateTimer, 1000);
}

function updateTimer() {
  const el = document.getElementById('timer');
  if (!el || !startTime) return;
  const elapsed = Math.floor((Date.now() - startTime) / 1000);
  const min = Math.floor(elapsed / 60);
  const sec = elapsed % 60;
  el.textContent = `${min}:${sec.toString().padStart(2, '0')}`;
}

function pollStatus(name) {
  fetch(`/api/status/${name}`)
    .then(r => r.json())
    .then(state => {
      renderState(state);
      if (state.phase === 'done' || state.phase === 'failed') {
        stopPolling();
        if (timerInterval) {
          clearInterval(timerInterval);
          timerInterval = null;
        }
        document.getElementById('btn-run').style.display = 'none';
        document.getElementById('btn-cleanup').style.display = 'inline-block';
      }
    })
    .catch(() => {});
}

function renderState(state) {
  const tbody = document.getElementById('agent-body');
  if (!tbody) return;

  tbody.innerHTML = '';

  let totalMem = 0, totalCpu = 0, totalIn = 0, totalOut = 0;

  state.agents.forEach(a => {
    const elapsed = agentElapsed(a);
    totalMem += a.memoryMiB || 0;
    totalCpu += a.cpuMcpu || 0;
    totalIn += a.inputTokens || 0;
    totalOut += a.outputTokens || 0;

    const row = document.createElement('tr');
    row.innerHTML = `
      <td>${escapeHtml(a.name)}</td>
      <td>${escapeHtml(a.label)}</td>
      <td><span class="status status-${statusClass(a.status)}">${statusText(a.status)}</span></td>
      <td>${elapsed}</td>
      <td>${a.memoryMiB ? a.memoryMiB.toFixed(1) : '-'}</td>
      <td>${a.cpuMcpu ? a.cpuMcpu.toFixed(0) : '-'}</td>
      <td>${a.inputTokens || '-'}</td>
      <td>${a.outputTokens || '-'}</td>
    `;
    tbody.appendChild(row);
  });

  // Total row
  const totalRow = document.createElement('tr');
  totalRow.className = 'total-row';
  totalRow.innerHTML = `
    <td colspan="4">Total (${state.agents.length} agents)</td>
    <td>${totalMem.toFixed(1)}</td>
    <td>${totalCpu.toFixed(0)}</td>
    <td>${totalIn}</td>
    <td>${totalOut}</td>
  `;
  tbody.appendChild(totalRow);

  // Update totals bar
  setMetric('total-mem', totalMem.toFixed(1) + ' MiB');
  setMetric('total-cpu', totalCpu.toFixed(0) + ' mcpu');
  setMetric('total-tokens', (totalIn + totalOut).toLocaleString());

  // Update doc-list with report links when agents complete.
  state.agents.forEach(a => {
    if (!a.documentId) return;
    const row = document.getElementById('doc-' + a.documentId);
    if (!row) return;
    const slot = row.querySelector('.doc-result');
    if (!slot) return;

    if (a.result) {
      slot.innerHTML = `<a class="doc-link report-link" href="#" onclick="showReport('${escapeJsString(a.name)}', '${escapeJsString(a.label)}'); return false;">View Report</a>`;
    } else if (a.status && a.status !== 'pending' && a.status !== 'deploying' && a.status !== 'ready') {
      slot.innerHTML = '<span class="report-pending">analyzing...</span>';
    }
  });

  // Store results for report viewing.
  window._agentResults = {};
  state.agents.forEach(a => {
    if (a.result) window._agentResults[a.name] = { label: a.label, result: a.result };
  });
}

function agentElapsed(a) {
  if (!a.startTime) return '-';
  const start = new Date(a.startTime);
  const end = a.endTime ? new Date(a.endTime) : new Date();
  const sec = Math.floor((end - start) / 1000);
  return sec + 's';
}

function statusText(s) {
  const map = {
    pending: '○ Pending',
    deploying: '◌ Deploying',
    ready: '● Ready',
    working: '◉ Working',
    completed: '✓ Ready',
    TASK_STATE_COMPLETED: '✓ Ready',
    failed: '✗ Failed',
    TASK_STATE_FAILED: '✗ Failed',
  };
  return map[s] || s;
}

function statusClass(s) {
  if (s === 'TASK_STATE_COMPLETED') return 'completed';
  if (s === 'TASK_STATE_FAILED') return 'failed';
  return s;
}

function setMetric(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

function cleanup(name) {
  const btn = document.getElementById('btn-cleanup');
  btn.disabled = true;
  btn.textContent = 'Cleaning up...';

  fetch(`/api/cleanup/${name}`, { method: 'POST' })
    .then(r => r.json())
    .then(() => {
      btn.textContent = 'Cleaned up';
      document.getElementById('btn-run').style.display = 'inline-block';
      document.getElementById('btn-run').disabled = false;
      document.getElementById('btn-run').textContent = 'Run';
      btn.style.display = 'none';
      startTime = null;
      document.getElementById('timer').textContent = '0:00';
      document.getElementById('agent-body').innerHTML = '';
      document.getElementById('results').innerHTML = '';
      setMetric('total-mem', '-');
      setMetric('total-cpu', '-');
      setMetric('total-tokens', '-');
    });
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

function escapeJsString(str) {
  return str.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n');
}

function showDocument(docId) {
  const viewer = document.getElementById('doc-viewer');
  const title = document.getElementById('doc-viewer-title');
  const content = document.getElementById('doc-viewer-content');

  title.textContent = docId + ' — loading...';
  content.textContent = '';
  viewer.style.display = 'block';

  fetch(`/api/document/${docId}`)
    .then(r => r.json())
    .then(data => {
      const doc = data.document || data;
      title.textContent = doc.title || docId;
      content.textContent = doc.content || JSON.stringify(doc, null, 2);
    })
    .catch(err => {
      title.textContent = docId + ' — error';
      content.textContent = err.message;
    });
}

function closeDocument() {
  document.getElementById('doc-viewer').style.display = 'none';
}

function showReport(agentName, label) {
  const data = window._agentResults && window._agentResults[agentName];
  if (!data) return;

  const viewer = document.getElementById('doc-viewer');
  const title = document.getElementById('doc-viewer-title');
  const content = document.getElementById('doc-viewer-content');

  title.textContent = 'Report — ' + data.label;
  content.textContent = data.result;
  viewer.style.display = 'block';
}
