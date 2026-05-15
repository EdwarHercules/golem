'use strict';

// ── DOM refs ────────────────────────────────────────────────────────────────
const codeInput  = document.getElementById('code-input');
const lineNums   = document.getElementById('line-numbers');
const btnAnalyze = document.getElementById('btn-analyze');
const btnSec     = document.getElementById('btn-security');
const btnExampleSec  = document.getElementById('btn-example-sec');
const btnExampleQual = document.getElementById('btn-example-qual');
const btnUpload  = document.getElementById('btn-upload');
const fileInput  = document.getElementById('file-upload');
const btnClear   = document.getElementById('btn-clear');
const outputArea = document.getElementById('output-area');
const statusDot  = document.getElementById('status-dot');
const statusTxt  = document.getElementById('status-text');
const cursorPos  = document.getElementById('cursor-pos');

// ── Example code ─────────────────────────────────────────────────────────────
// Security example: SQL injection + hardcoded credentials
const EXAMPLE = `package main

import (
\t"database/sql"
\t"fmt"
\t"net/http"
)

var db *sql.DB

func getUser(w http.ResponseWriter, r *http.Request) {
\t// Vulnerable: SQL injection
\tid := r.URL.Query().Get("id")
\tquery := "SELECT * FROM users WHERE id = " + id
\trows, _ := db.Query(query)
\tdefer rows.Close()
\tfmt.Fprintf(w, "%v", rows)
}

func main() {
\tpassword := "admin123" // hardcoded credential
\tfmt.Println("DB password:", password)

\thttp.HandleFunc("/user", getUser)
\thttp.ListenAndServe(":8080", nil)
}`;

// Quality example: high cyclomatic complexity + long nested functions
const EXAMPLE_QUALITY = `package main

import "fmt"

func processOrder(status string, items []string,
  userAge int, isPremium bool, hasDiscount bool) string {
  result := ""
  if status == "active" {
    if userAge >= 18 {
      if len(items) > 0 {
        for _, item := range items {
          if item != "" {
            if isPremium {
              if hasDiscount {
                result += "PREMIUM_DISCOUNT:" + item + ";"
              } else {
                result += "PREMIUM:" + item + ";"
              }
            } else {
              if hasDiscount {
                result += "DISCOUNT:" + item + ";"
              } else {
                result += "NORMAL:" + item + ";"
              }
            }
          }
        }
      } else {
        result = "NO_ITEMS"
      }
    } else {
      result = "UNDERAGE"
    }
  } else if status == "pending" {
    result = "PENDING"
  } else if status == "cancelled" {
    result = "CANCELLED"
  } else {
    result = "UNKNOWN"
  }
  return result
}

func calculateTotal(prices []float64, tax float64,
  discount float64, currency string) float64 {
  total := 0.0
  for _, p := range prices {
    total += p
  }
  if discount > 0 {
    total = total - (total * discount / 100)
  }
  if tax > 0 {
    total = total + (total * tax / 100)
  }
  if currency == "USD" {
    return total
  } else if currency == "EUR" {
    return total * 0.92
  } else if currency == "GBP" {
    return total * 0.79
  }
  return total
}

func main() {
  items := []string{"laptop", "mouse", "keyboard"}
  result := processOrder("active", items, 25, true, true)
  fmt.Println(result)
  total := calculateTotal([]float64{100, 200, 300},
    15, 10, "USD")
  fmt.Printf("Total: %.2f\\n", total)
}`;

// ── State ─────────────────────────────────────────────────────────────────────
let running      = false;
let lastCode     = '';
let lastFindings = [];
let codeSnippets = []; // accumulated code_generated events

// ── Placeholder ───────────────────────────────────────────────────────────────
codeInput.placeholder = '// Pega tu código Go aquí...\n//\n// O usa los botones de ejemplo para una demo';

// ── Line numbers ──────────────────────────────────────────────────────────────
function syncLineNumbers() {
  const count   = codeInput.value.split('\n').length;
  const current = lineNums.children.length;
  if (count > current) {
    const frag = document.createDocumentFragment();
    for (let i = current + 1; i <= count; i++) {
      const li = document.createElement('li');
      li.textContent = i;
      frag.appendChild(li);
    }
    lineNums.appendChild(frag);
  } else {
    while (lineNums.children.length > count) lineNums.removeChild(lineNums.lastChild);
  }
  lineNums.scrollTop = codeInput.scrollTop;
}

// ── Cursor ────────────────────────────────────────────────────────────────────
function updateCursor() {
  const before = codeInput.value.slice(0, codeInput.selectionStart);
  const lines  = before.split('\n');
  cursorPos.textContent = `Ln ${lines.length}, Col ${lines[lines.length - 1].length + 1}`;
}

// ── Status bar ────────────────────────────────────────────────────────────────
function setStatus(state, text) {
  statusDot.className = 'status-dot' + (state ? ` ${state}` : '');
  statusTxt.textContent = text;
}

// ── Output helpers ────────────────────────────────────────────────────────────
function clearOutput() { outputArea.innerHTML = ''; }

function restoreEmptyState() {
  clearOutput();
  const el    = document.createElement('div');
  el.className = 'empty-state';
  el.id        = 'empty-state';
  const glyph = document.createElement('span');
  glyph.className = 'empty-glyph';
  glyph.setAttribute('aria-hidden', 'true');
  glyph.textContent = '◆';
  const p = document.createElement('p');
  p.textContent = 'Pega código Go en el editor y ejecuta un análisis';
  el.append(glyph, p);
  outputArea.appendChild(el);
}

// ── Section A: progress log ───────────────────────────────────────────────────
function createLiveSection() {
  const wrap = document.createElement('div');
  wrap.className = 'live-section';

  const sectionHead = document.createElement('div');
  sectionHead.className = 'section-header';

  const label = document.createElement('div');
  label.className   = 'section-label';
  label.textContent = 'Progreso en vivo';

  const toggle = document.createElement('button');
  toggle.className    = 'detail-toggle';
  toggle.textContent  = '▶ Resumido';
  toggle.dataset.mode = 'summary';

  sectionHead.append(label, toggle);

  const log = document.createElement('div');
  log.className = 'progress-log';

  toggle.addEventListener('click', () => {
    if (toggle.dataset.mode === 'summary') {
      toggle.dataset.mode = 'detail';
      toggle.textContent  = '▶ Detallado';
      log.classList.add('detail-mode');
    } else {
      toggle.dataset.mode = 'summary';
      toggle.textContent  = '▶ Resumido';
      log.classList.remove('detail-mode');
    }
  });

  wrap.append(sectionHead, log);
  outputArea.appendChild(wrap);
  return log;
}

// icon + color per SSE event type
const EV_META = {
  progress:   { icon: '⚙', cls: 'ev-progress'   },
  step_start: { icon: '→', cls: 'ev-step-start'  },
  step_done:  { icon: '✓', cls: 'ev-step-done'   },
  retry:      { icon: '↻', cls: 'ev-retry'       },
  error:      { icon: '✗', cls: 'ev-error'        },
};

// Event types shown only in detail mode (hidden in summary)
const DETAIL_ONLY = new Set(['step_start', 'retry']);

function addLogEvent(log, type, message) {
  const meta = EV_META[type] || EV_META.progress;
  const item = document.createElement('div');
  item.className = `progress-item ${meta.cls}`;
  if (DETAIL_ONLY.has(type)) item.dataset.detailOnly = 'true';

  const icon = document.createElement('span');
  icon.className = 'ev-icon';
  icon.setAttribute('aria-hidden', 'true');
  icon.textContent = meta.icon;

  const msg = document.createElement('span');
  msg.className = 'p-msg';
  msg.textContent = message;

  item.append(icon, msg);
  log.appendChild(item);
  item.scrollIntoView({ block: 'nearest' });
}

// ── Section B: result tabs ────────────────────────────────────────────────────
function createResultSection() {
  const sec    = document.createElement('div');
  sec.className = 'result-section';

  const tabBar = document.createElement('div');
  tabBar.className = 'tab-bar';

  const defs = [
    { id: 'tab-findings', label: 'Hallazgos'        },
    { id: 'tab-report',   label: 'Reporte'           },
    { id: 'tab-code',     label: 'Código generado'   },
  ];

  const panels = {};
  defs.forEach(({ id, label }, i) => {
    const btn = document.createElement('button');
    btn.className    = 'tab-btn' + (i === 0 ? ' active' : '');
    btn.textContent  = label;
    btn.dataset.tab  = id;
    btn.addEventListener('click', () => switchTab(sec, id));
    tabBar.appendChild(btn);

    const panel = document.createElement('div');
    panel.id        = id;
    panel.className = 'tab-panel' + (i === 0 ? ' active' : '');
    panels[id] = panel;
    sec.appendChild(panel);
  });

  sec.insertBefore(tabBar, sec.firstChild);
  return { sec, panels };
}

function switchTab(container, activeId) {
  container.querySelectorAll('.tab-btn').forEach(b =>
    b.classList.toggle('active', b.dataset.tab === activeId));
  container.querySelectorAll('.tab-panel').forEach(p =>
    p.classList.toggle('active', p.id === activeId));
}

// ── Findings renderer ─────────────────────────────────────────────────────────

// Maps Spanish and English severity labels to CSS class names.
function normalizeSev(sev) {
  const map = {
    'crítico': 'critical', 'critico': 'critical',
    'alto':    'high',
    'medio':   'medium',   'aceptable': 'medium',
    'bajo':    'low',
  };
  const s = (sev || '').toLowerCase();
  return map[s] || s || 'low';
}

function buildSecurityCard(f) {
  const sev  = normalizeSev(f.severity);
  const card = document.createElement('div');
  card.className = `finding-card ${sev}`;

  const meta = document.createElement('div');
  meta.className = 'finding-meta';

  const badge = document.createElement('span');
  badge.className   = `badge badge-${sev}`;
  badge.textContent = (f.severity || sev).toUpperCase();

  const title = document.createElement('span');
  title.className   = 'finding-type';
  title.textContent = f.type || 'Hallazgo';

  meta.append(badge, title);

  if (f.line) {
    const lineEl = document.createElement('span');
    lineEl.className   = 'finding-line';
    lineEl.textContent = `Línea ${f.line}`;
    meta.appendChild(lineEl);
  }
  card.appendChild(meta);

  if (f.description) {
    const desc = document.createElement('p');
    desc.className   = 'finding-desc';
    desc.textContent = f.description;
    card.appendChild(desc);
  }

  if (f.code_snippet) {
    const snip = document.createElement('div');
    snip.className   = 'finding-snippet';
    snip.textContent = f.code_snippet;
    card.appendChild(snip);
  }
  return card;
}

function buildComplexityCard(f) {
  const sev  = normalizeSev(f.severity);
  const card = document.createElement('div');
  card.className = `finding-card ${sev}`;

  const meta = document.createElement('div');
  meta.className = 'finding-meta';

  const badge = document.createElement('span');
  badge.className   = `badge badge-${sev}`;
  badge.textContent = (f.severity || sev).toUpperCase();

  const title = document.createElement('span');
  title.className   = 'finding-type';
  title.textContent = `${f.function}() — complejidad ${f.complexity}`;

  meta.append(badge, title);
  card.appendChild(meta);

  const desc = document.createElement('p');
  desc.className   = 'finding-desc';
  desc.textContent = `Umbral recomendado: ${f.threshold || 10}`;
  card.appendChild(desc);

  return card;
}

function buildQualityCheckCard(f) {
  const isOk = f.status === 'success';
  const card = document.createElement('div');
  card.className = `finding-card ${isOk ? 'ok' : 'critical'}`;

  const meta = document.createElement('div');
  meta.className = 'finding-meta';

  const badge = document.createElement('span');
  badge.className   = `badge badge-${isOk ? 'ok' : 'critical'}`;
  badge.textContent = isOk ? 'OK' : 'ERROR';

  const title = document.createElement('span');
  title.className   = 'finding-type';
  title.textContent = f.message || f.type;

  meta.append(badge, title);
  card.appendChild(meta);

  return card;
}

function renderFindings(findings, parent) {
  if (!findings || findings.length === 0) {
    const none = document.createElement('p');
    none.className = 'tab-empty';
    none.textContent = 'No se encontraron hallazgos.';
    parent.appendChild(none);
    return;
  }

  const list = document.createElement('div');
  list.className = 'findings-list';

  for (const f of findings) {
    let card;
    if (f.function !== undefined && f.complexity !== undefined) {
      card = buildComplexityCard(f);
    } else if (f.status !== undefined && f.message !== undefined) {
      card = buildQualityCheckCard(f);
    } else {
      card = buildSecurityCard(f);
    }
    list.appendChild(card);
  }
  parent.appendChild(list);
}

// ── Markdown renderer (no libraries) ─────────────────────────────────────────
function escHtml(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function inlineMarkdown(raw) {
  let s = escHtml(raw);
  s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  s = s.replace(/`([^`]+)`/g, '<code>$1</code>');
  return s;
}

function renderMarkdown(text) {
  const lines = text.split('\n');
  let html = '';
  let inCode = false;
  let codeLang = '';
  let codeLines = [];

  for (const raw of lines) {
    if (!inCode && raw.startsWith('```')) {
      inCode = true;
      codeLang = raw.slice(3).trim();
      codeLines = [];
      continue;
    }
    if (inCode) {
      if (raw.trimEnd() === '```') {
        inCode = false;
        const cls = codeLang ? ` class="lang-${escHtml(codeLang)}"` : '';
        html += `<pre><code${cls}>${escHtml(codeLines.join('\n'))}</code></pre>`;
        codeLines = []; codeLang = '';
      } else {
        codeLines.push(raw);
      }
      continue;
    }

    const trimmed = raw.trim();
    if (!trimmed) continue;

    if (/^-{3,}$/.test(trimmed)) { html += '<hr>'; continue; }

    if (trimmed === '**RESUMEN**') {
      html += '<div class="report-summary"><strong>RESUMEN</strong></div>';
      continue;
    }
    if (raw.startsWith('### ')) { html += `<h4>${inlineMarkdown(raw.slice(4))}</h4>`; continue; }
    if (raw.startsWith('## '))  { html += `<h3>${inlineMarkdown(raw.slice(3))}</h3>`; continue; }

    if (raw.startsWith('🔴')) { html += `<div class="report-finding critical">${inlineMarkdown(raw)}</div>`; continue; }
    if (raw.startsWith('🟡')) { html += `<div class="report-finding medium">${inlineMarkdown(raw)}</div>`;   continue; }
    if (raw.startsWith('🟢')) { html += `<div class="report-finding low">${inlineMarkdown(raw)}</div>`;      continue; }

    html += `<p>${inlineMarkdown(raw)}</p>`;
  }

  if (inCode) {
    const cls = codeLang ? ` class="lang-${escHtml(codeLang)}"` : '';
    html += `<pre><code${cls}>${escHtml(codeLines.join('\n'))}</code></pre>`;
  }
  return html;
}

// ── Report renderer ───────────────────────────────────────────────────────────
function renderReport(report, parent) {
  if (!report || !report.trim()) {
    const none = document.createElement('p');
    none.className   = 'tab-empty';
    none.textContent = 'Sin reporte generado.';
    parent.appendChild(none);
    return;
  }
  const body = document.createElement('div');
  body.className  = 'report-body';
  body.innerHTML  = renderMarkdown(report);
  parent.appendChild(body);
}

// ── Generated code renderer ───────────────────────────────────────────────────
function renderGeneratedCode(snippets, parent) {
  if (!snippets || snippets.length === 0) {
    const none = document.createElement('p');
    none.className   = 'tab-empty';
    none.textContent = 'El agente no generó código en esta sesión.';
    parent.appendChild(none);
    return;
  }
  snippets.forEach((code, i) => {
    const wrap   = document.createElement('div');
    wrap.className = 'code-block';

    const header = document.createElement('div');
    header.className = 'code-block-header';

    const lbl = document.createElement('span');
    lbl.textContent = `Paso ${i + 1}`;

    const copyBtn = document.createElement('button');
    copyBtn.className   = 'copy-btn';
    copyBtn.textContent = 'Copiar';
    copyBtn.addEventListener('click', () => {
      navigator.clipboard.writeText(code).then(() => {
        copyBtn.textContent = '✓ Copiado';
        setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 1500);
      });
    });

    header.append(lbl, copyBtn);

    const pre = document.createElement('pre');
    pre.className   = 'code-block-body';
    pre.textContent = code;

    wrap.append(header, pre);
    parent.appendChild(wrap);
  });
}

// ── Fix button ────────────────────────────────────────────────────────────────
function createFixButton(findings, resultSec) {
  const btn = document.createElement('button');
  btn.className = 'btn btn-fix';
  btn.innerHTML = '&#128295; Aplicar Fix';
  btn.addEventListener('click', () => runFix(findings, resultSec, btn));
  resultSec.appendChild(btn);
}

// ── Diff renderer ─────────────────────────────────────────────────────────────
function renderDiff(patches, parent) {
  if (!patches || patches.length === 0) return;

  const sec   = document.createElement('div');
  sec.className = 'diff-section';

  const label = document.createElement('div');
  label.className   = 'section-label';
  label.textContent = 'Cambios aplicados';
  sec.appendChild(label);

  for (const p of patches) {
    const block = document.createElement('div');
    block.className = 'diff-block';
    if (p.original) {
      const rem = document.createElement('pre');
      rem.className   = 'diff-line diff-remove';
      rem.textContent = '- ' + p.original;
      block.appendChild(rem);
    }
    if (p.fixed) {
      const add = document.createElement('pre');
      add.className   = 'diff-line diff-add';
      add.textContent = '+ ' + p.fixed;
      block.appendChild(add);
    }
    sec.appendChild(block);
  }
  parent.appendChild(sec);
}

// ── Error banner ──────────────────────────────────────────────────────────────
function showError(message) {
  const el = document.createElement('div');
  el.className = 'error-msg';
  el.setAttribute('role', 'alert');
  const icon = document.createElement('span');
  icon.className = 'error-icon';
  icon.setAttribute('aria-hidden', 'true');
  icon.textContent = '✕';
  const txt = document.createElement('span');
  txt.textContent = message;
  el.append(icon, txt);
  outputArea.appendChild(el);
}

// ── SSE stream reader ─────────────────────────────────────────────────────────
// EventSource only supports GET. POST endpoints use ReadableStream + manual parsing.
async function readSSE(response, onEvent) {
  const reader  = response.body.getReader();
  const decoder = new TextDecoder();
  let   buf     = '';
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const parts = buf.split('\n\n');
      buf = parts.pop();
      for (const part of parts) {
        for (const line of part.split('\n')) {
          if (!line.startsWith('data: ')) continue;
          try { onEvent(JSON.parse(line.slice(6))); } catch { /* skip malformed */ }
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

// ── Fix flow ──────────────────────────────────────────────────────────────────
async function runFix(findings, resultSec, btn) {
  btn.disabled    = true;
  btn.textContent = '⏳ Aplicando...';
  setStatus('busy', 'Generando fixes...');

  const fixLog = createLiveSection();
  addLogEvent(fixLog, 'progress', 'Iniciando agente de fix...');

  try {
    const res = await fetch('/fix', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({ code: lastCode, findings }),
    });
    if (!res.ok) {
      const payload = await res.json().catch(() => ({}));
      throw new Error(payload.error || `HTTP ${res.status}`);
    }

    await readSSE(res, (ev) => {
      switch (ev.type) {
        case 'progress':
          addLogEvent(fixLog, 'progress', ev.message);
          break;
        case 'done':
          addLogEvent(fixLog, 'step_done', 'Fix generado');
          if (ev.fixed_code) {
            codeInput.value = ev.fixed_code;
            syncLineNumbers();
          }
          renderDiff(ev.patches, resultSec);
          setStatus('', 'Fix aplicado');
          btn.remove();
          break;
        case 'error':
          addLogEvent(fixLog, 'error', ev.message);
          setStatus('error', 'Error en fix');
          btn.disabled    = false;
          btn.textContent = '🔧 Aplicar Fix';
          break;
      }
    });
  } catch (err) {
    addLogEvent(fixLog, 'error', err.message);
    setStatus('error', 'Error');
    btn.disabled    = false;
    btn.textContent = '🔧 Aplicar Fix';
  }
}

// ── Main analysis flow ────────────────────────────────────────────────────────
async function runAnalysis(endpoint, label) {
  const code = codeInput.value.trim();
  if (!code || running) return;

  running      = true;
  lastCode     = code;
  lastFindings = [];
  codeSnippets = [];

  btnAnalyze.disabled = true;
  btnSec.disabled     = true;
  btnClear.classList.add('hidden');

  clearOutput();
  const log = createLiveSection();
  let resultSec, panels;

  setStatus('busy', `${label}...`);
  addLogEvent(log, 'progress', 'Conectando con el agente...');

  try {
    const res = await fetch(endpoint, {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({ code }),
    });
    if (!res.ok) {
      const payload = await res.json().catch(() => ({}));
      throw new Error(payload.error || `HTTP ${res.status}`);
    }

    await readSSE(res, (ev) => {
      switch (ev.type) {
        case 'progress':
          addLogEvent(log, 'progress', ev.message);
          break;

        case 'step_start':
          addLogEvent(log, 'step_start', ev.message);
          break;

        case 'step_done':
          addLogEvent(log, 'step_done', ev.message);
          break;

        case 'retry':
          addLogEvent(log, 'retry', ev.message);
          break;

        case 'code_generated':
          if (ev.code) codeSnippets.push(ev.code);
          break;

        case 'done': {
          addLogEvent(log, 'step_done', 'Análisis completado');
          lastFindings = ev.findings || [];

          const created = createResultSection();
          resultSec     = created.sec;
          panels        = created.panels;
          outputArea.appendChild(resultSec);

          // Update findings tab label with count when non-zero
          if (lastFindings.length > 0) {
            const findBtn = resultSec.querySelector('[data-tab="tab-findings"]');
            if (findBtn) findBtn.textContent = `Hallazgos (${lastFindings.length})`;
          }

          renderFindings(lastFindings, panels['tab-findings']);
          renderReport(ev.report, panels['tab-report']);
          renderGeneratedCode(codeSnippets, panels['tab-code']);

          // Fix button only for security audits that found something
          if (endpoint === '/security' && lastFindings.length > 0) {
            createFixButton(lastFindings, resultSec);
          }

          setStatus('', 'Listo');
          btnClear.classList.remove('hidden');
          break;
        }

        case 'error':
          addLogEvent(log, 'error', ev.message);
          setStatus('error', 'Error');
          break;
      }
    });

  } catch (err) {
    addLogEvent(log, 'error',
      err.message.includes('fetch')
        ? 'No se pudo conectar con el servidor. ¿Está corriendo golem serve?'
        : err.message);
    setStatus('error', 'Error');
  } finally {
    running             = false;
    const hasCode       = codeInput.value.trim() !== '';
    btnAnalyze.disabled = !hasCode;
    btnSec.disabled     = !hasCode;
  }
}

// ── Event listeners ───────────────────────────────────────────────────────────
codeInput.addEventListener('input', () => {
  syncLineNumbers();
  const hasCode       = codeInput.value.trim() !== '';
  btnAnalyze.disabled = !hasCode || running;
  btnSec.disabled     = !hasCode || running;
});

codeInput.addEventListener('scroll', () => { lineNums.scrollTop = codeInput.scrollTop; });
codeInput.addEventListener('keyup',  updateCursor);
codeInput.addEventListener('click',  updateCursor);
codeInput.addEventListener('select', updateCursor);

codeInput.addEventListener('keydown', (e) => {
  if (e.key !== 'Tab') return;
  e.preventDefault();
  const s = codeInput.selectionStart;
  codeInput.value =
    codeInput.value.slice(0, s) + '\t' + codeInput.value.slice(codeInput.selectionEnd);
  codeInput.selectionStart = codeInput.selectionEnd = s + 1;
  syncLineNumbers();
});

btnAnalyze.addEventListener('click', () => runAnalysis('/analyze',  'Analizando'));
btnSec.addEventListener('click',     () => runAnalysis('/security', 'Auditando seguridad'));

btnExampleSec.addEventListener('click', () => {
  codeInput.value = EXAMPLE;
  codeInput.dispatchEvent(new Event('input'));
  syncLineNumbers();
  updateCursor();
  btnAnalyze.disabled = false;
  btnSec.disabled     = false;
  codeInput.focus();
});

btnExampleQual.addEventListener('click', () => {
  codeInput.value = EXAMPLE_QUALITY;
  codeInput.dispatchEvent(new Event('input'));
  syncLineNumbers();
  updateCursor();
  btnAnalyze.disabled = false;
  btnSec.disabled     = false;
  codeInput.focus();
});

btnUpload.addEventListener('click', () => fileInput.click());

fileInput.addEventListener('change', () => {
  const file = fileInput.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = (e) => {
    codeInput.value = e.target.result;
    syncLineNumbers();
    updateCursor();
    btnAnalyze.disabled = false;
    btnSec.disabled     = false;
    codeInput.focus();
  };
  reader.readAsText(file);
  fileInput.value = ''; // allow re-selecting the same file
});

btnClear.addEventListener('click', () => {
  restoreEmptyState();
  btnClear.classList.add('hidden');
  setStatus('', 'Listo');
});

// ── Init ──────────────────────────────────────────────────────────────────────
syncLineNumbers();
