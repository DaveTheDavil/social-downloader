package api

// adsPanelHTML — jednoplikowy panel webowy (HTML+CSS+JS, brak zależności
// zewnętrznych). Uwaga: string w Go-języku `raw` — w środku MOGĄ być
// cudzysłowy, ale nie może być grave-accentu (`), więc JS pisany bez
// template literals (tylko zwykłe konkatenacje).
const adsPanelHTML = `<!DOCTYPE html>
<html lang="pl">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Social Downloader — Panel reklamodawców</title>
<style>
  :root {
    --bg: #0F0C18; --card: #191330; --card2: #241A44; --line: #2B2050;
    --text: #E6DEFA; --dim: #9A92B8; --accent: #FF2E6D; --ok: #38E07B;
    --warn: #F5C400; --err: #FF6B6B;
  }
  * { box-sizing: border-box; }
  body { margin: 0; font: 14px/1.45 "Segoe UI", system-ui, sans-serif; background: var(--bg); color: var(--text); }
  header { display: flex; align-items: baseline; gap: 16px; padding: 14px 20px 10px; border-bottom: 1px solid var(--line); }
  header h1 { margin: 0; font-size: 20px; letter-spacing: .5px; text-transform: uppercase; }
  header .sub { color: var(--dim); font-size: 12px; }
  #wrap { max-width: 980px; margin: 0 auto; padding: 14px 20px 60px; }
  .badge-prem { display: none; margin: 10px 0; padding: 10px 12px; border: 1px solid var(--warn); border-radius: 8px; color: var(--warn); font-size: 12.5px; }
  .hint { color: var(--dim); font-size: 12.5px; margin: 10px 2px 18px; }
  section { margin: 0 0 26px; }
  h2 { margin: 0 0 10px; font-size: 16px; letter-spacing: 1px; text-transform: uppercase; color: var(--text); border-bottom: 2px solid var(--accent); display: inline-block; padding-bottom: 2px; }
  .card { background: var(--card); border: 1px solid var(--line); border-radius: 10px; padding: 12px 14px; margin-bottom: 12px; }
  .row { display: grid; grid-template-columns: 130px 1fr; gap: 8px 12px; align-items: center; }
  .row label { color: var(--dim); font-size: 12px; }
  .row input[type=text], .row input[type=number] {
    width: 100%; background: #0D0818; color: var(--text); border: 1px solid var(--line);
    border-radius: 6px; padding: 6px 8px; font: inherit;
  }
  .row input:focus { border-color: var(--accent); outline: none; }
  .top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
  .top .id { color: var(--accent); font-weight: 600; }
  .top .meta { color: var(--dim); font-size: 12px; }
  button { background: var(--card2); color: var(--text); border: 1px solid var(--line); border-radius: 6px; padding: 6px 12px; cursor: pointer; font: inherit; }
  button:hover { border-color: var(--accent); }
  button.primary { background: var(--accent); border-color: var(--accent); color: #fff; font-weight: 600; }
  button.ghost { background: transparent; }
  .preview { display: flex; gap: 12px; align-items: flex-start; }
  .preview img { max-width: 240px; max-height: 90px; border: 1px solid var(--line); border-radius: 6px; background: #0D0818; }
  .nofile { color: var(--dim); font-size: 12px; }
  .actions { position: sticky; bottom: 0; background: linear-gradient(transparent, var(--bg) 30%); padding: 16px 0; display: flex; gap: 10px; align-items: center; }
  #msg { font-size: 13px; white-space: pre-line; }
  #msg.ok { color: var(--ok); }
  #msg.warn { color: var(--warn); }
  #msg.err { color: var(--err); }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 0 12px; }
  input[type=file] { color: var(--dim); font-size: 12px; max-width: 220px; }
</style>
</head>
<body>
<header>
  <h1>Panel reklamodawców</h1>
  <div class="sub">ads-config.json · prefiks <b id="engineAddr">127.0.0.1:8765</b></div>
</header>
<div id="wrap">
  <div class="badge-prem" id="premNote">⚠️ Na TEN komputer działa licencja premium — reklamy tu nie są losowane. Konfiguracja nadal zapisuje się do ads-config.json i obowiązuje w paczkach rozsyłanych użytkownikom (tryb darmowy).</div>
  <p class="hint">Zmiany → „Zapisz”: walidacja, backup <i>ads-config.json.bak</i> i podmiana w pamięci silnika (efekt hot — bez restartu). Wagi = udział w losowaniu, pokazywany w %. Kreatywy wgrywane lądują w katalogu obrazów obok silnika/klienta.</p>

  <section>
    <h2>Banery</h2>
    <div id="banners"></div>
    <button onclick="addUnit('banners')">+ dodaj baner</button>
  </section>

  <section>
    <h2>Interstitials (produkowe, z licznikiem sekund)</h2>
    <div id="interstitials"></div>
    <button onclick="addUnit('interstitials')">+ dodaj interstitial</button>
  </section>

  <div class="actions">
    <button class="primary" onclick="saveAll()">💾 Zapisz</button>
    <button class="ghost" onclick="loadFromEngine(true)">Odtwórz z dysku</button>
    <span id="msg"></span>
  </div>
</div>

<script>
'use strict';
var state = { banners: [], interstitials: [] };

function esc(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function totalWeight(typ) {
  var t = 0;
  for (var i = 0; i < state[typ].length; i++) t += (state[typ][i].weight > 0 ? state[typ][i].weight : 0);
  return t;
}

function nextId(typ) {
  var pre = typ === 'banners' ? 'ban-' : 'int-';
  var max = 0;
  var all = state.banners.concat(state.interstitials);
  for (var i = 0; i < all.length; i++) {
    var m = all[i].id.match(new RegExp('^' + pre + '(\\d+)$'));
    if (m) { var n = parseInt(m[1], 10); if (n > max) max = n; }
  }
  return pre + (max + 1);
}

function loadFromEngine(announce) {
  var msg = document.getElementById('msg');
  fetch('/ads-config').then(function (r) { return r.json(); }).then(function (j) {
    state.banners = j.banners || [];
    state.interstitials = j.interstitials || [];
    for (var i = 0; i < state.banners.length; i++) {
      if (!(state.banners[i].weight > 0)) state.banners[i].weight = 1;
    }
    for (var k = 0; k < state.interstitials.length; k++) {
      if (!(state.interstitials[k].weight > 0)) state.interstitials[k].weight = 1;
    }
    render();
    if (j.file) {
      document.querySelector('.sub').innerHTML = 'plik konfiguracji: <b>' + esc(j.file) + '</b>';
    }
    document.getElementById('premNote').style.display = j.premium ? 'block' : 'none';
    if (announce) msg.textContent = 'Wczytano z silnika (lokalne zmiany porzucone).';
  }, function (e) {
    msg.textContent = 'Nie mogę wczytać konfiguracji: ' + e;
    msg.className = 'err';
  });
}

function onInput(typ, i, field, value) {
  if (!state[typ][i]) return;
  if (field === 'weight') {
    var w = parseInt(value, 10);
    state[typ][i].weight = isNaN(w) ? 0 : w;
    refreshPct(typ, i);
  } else {
    state[typ][i][field] = value;
    if (field === 'image') refreshImg(typ, i);
  }
}

function refreshPct(typ, i) {
  var el = document.getElementById('pct-' + typ + '-' + i);
  if (!el) return;
  var t = totalWeight(typ);
  var w = state[typ][i].weight > 0 ? state[typ][i].weight : 0;
  el.textContent = t > 0 && w > 0 ? Math.round(w / t * 1000) / 10 + '%' : '0%';
}

function imgSrc(path) {
  var p = String(path || '');
  if (!p) return '';
  var low = p.toLowerCase();
  if (low.indexOf('http://') === 0 || low.indexOf('https://') === 0) return p;
  var slash = p.lastIndexOf('/');
  var file = slash >= 0 ? p.slice(slash + 1) : p;
  return '/adsimg?file=' + encodeURIComponent(file);
}

function refreshImg(typ, i) {
  var el = document.getElementById('img-' + typ + '-' + i);
  if (!el) return;
  var src = imgSrc(state[typ][i].image);
  if (src) { el.src = src; el.style.display = 'inline-block'; }
  else { el.removeAttribute('src'); el.style.display = 'none'; }
}

function row(typ, u, i) {
  var d = document.createElement('div');
  d.className = 'card';
  var isInt = typ === 'interstitials';
  d.innerHTML =
    '<div class="top">' +
      '<span class="id">' + esc(u.id) + '</span>' +
      '<span class="meta">udział w losowaniu: <b id="pct-' + typ + '-' + i + '">–</b></span>' +
    '</div>' +
    '<div class="row">' +
      '<label>Reklamodawca</label><input type="text" maxlength="64" value="' + esc(u.advertiser) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'advertiser\', this.value)">' +
      '<label>Tytuł</label><input type="text" maxlength="120" value="' + esc(u.title) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'title\', this.value)">' +
      '<label>Opis</label><input type="text" maxlength="240" value="' + esc(u.description) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'description\', this.value)">' +
      '<label>URL docelowy</label><input type="text" maxlength="512" value="' + esc(u.url) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'url\', this.value)">' +
      '<label>Obraz (adsimg/…)</label><input type="text" maxlength="140" value="' + esc(u.image) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'image\', this.value)">' +
      '<label>Waga</label><input type="number" min="1" max="1000" step="1" value="' + esc(u.weight) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'weight\', this.value)">' +
      (isInt ? '<label>Sekundy ekspozycji</label><input type="number" min="1" max="60" step="1" value="' + esc(u.seconds) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'seconds\', this.value)">' : '') +
      '<label>Clickthrough (opcj.)</label><input type="text" maxlength="200" value="' + esc(u.clickthrough) + '" oninput="onInput(\'' + typ + '\',' + i + ',\'clickthrough\', this.value)">' +
    '</div>' +
    '<div class="preview" style="margin-top:10px">' +
      '<img id="img-' + typ + '-' + i + '" alt="podgląd">' +
      '<div>' +
        '<div style="margin-bottom:6px">Wgraj kreatywę:</div>' +
        '<input type="file" accept=".png,.jpg,.jpeg,.webp" onchange="upload(this, \'' + typ + '\', ' + i + ')">' +
      '</div>' +
      '<div style="flex:1"></div>' +
      '<button class="ghost" onclick="delUnit(\'' + typ + '\',' + i + ')">🗑 usuń</button>' +
    '</div>';
  return d;
}

function render() {
  renderList('banners');
  renderList('interstitials');
}

function renderList(typ) {
  var host = document.getElementById(typ);
  host.innerHTML = '';
  if (!state[typ].length) {
    host.innerHTML = '<p class="nofile">Brak pozycji — dodaj pierwszą poniżej.</p>';
  }
  for (var i = 0; i < state[typ].length; i++) host.appendChild(row(typ, state[typ][i], i));
  for (var j = 0; j < state[typ].length; j++) { refreshPct(typ, j); refreshImg(typ, j); }
}

function addUnit(typ) {
  var u = { id: nextId(typ), advertiser: '', title: '', description: '', url: 'https://example.com/', image: '', weight: 10 };
  if (typ === 'interstitials') { u.seconds = 5; }
  state[typ].push(u);
  renderList(typ);
}

function delUnit(typ, i) {
  if (!confirm('Usunąć pozycję ' + state[typ][i].id + ' (' + (state[typ][i].advertiser || 'bez nazwy') + ')?'))
    return;
  state[typ].splice(i, 1);
  renderList(typ);
}

function niceName(file) {
  var dot = file.lastIndexOf('.');
  var base = dot > 0 ? file.slice(0, dot) : file;
  var ext = dot > 0 ? file.slice(dot + 1).toLowerCase() : 'png';
  if (['png', 'jpg', 'jpeg', 'webp'].indexOf(ext) < 0) ext = 'png';
  base = base.toLowerCase().replace(/[^a-z0-9_.-]+/g, '-').replace(/^[^a-z0-9]+/g, '').slice(0, 60);
  if (!base) base = 'kreatywa';
  return base + '.' + ext;
}

function upload(input, typ, i) {
  var f = input.files && input.files[0];
  if (!f) return;
  var msg = document.getElementById('msg');
  var reader = new FileReader();
  reader.onload = function () {
    var b64 = String(reader.result).split(',')[1] || '';
    fetch('/ads-image', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: niceName(f.name), data: b64 })
    }).then(function (r) {
      r.json().then(function (j) {
        if (r.status === 200 && j.ok) {
          state[typ][i].image = j.path;
          renderList(typ);
          msg.textContent = 'Obraz zapisany jako ' + j.path + '.';
          msg.className = 'ok';
        } else {
          msg.textContent = 'Upload odrzucony: ' + (j.error || ('HTTP ' + r.status));
          msg.className = 'err';
        }
      }, function () {
        msg.textContent = 'Upload: odpowiedź nie jest JSON-em (HTTP ' + r.status + ')';
        msg.className = 'err';
      });
    }, function (e) {
      msg.textContent = 'Błąd sieci przy uploadzie: ' + e;
      msg.className = 'err';
    });
  };
  reader.readAsDataURL(f);
}

function saveAll() {
  var msg = document.getElementById('msg');
  msg.textContent = 'Zapisuję…';
  msg.className = '';
  fetch('/ads-config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ banners: state.banners, interstitials: state.interstitials })
  }).then(function (r) {
    r.json().then(function (j) {
      if (r.status === 200 && j.ok) {
        state.banners = j.banners || state.banners;
        state.interstitials = j.interstitials || state.interstitials;
        render();
        var txt = 'Zapisano ✓ (backup: ads-config.json.bak)';
        if (j.warnings && j.warnings.length) {
          txt += '\n⚠ Uwagi:\n' + j.warnings.join('\n');
          msg.className = 'warn';
        } else {
          msg.className = 'ok';
        }
        msg.textContent = txt;
      } else {
        msg.textContent = 'Walidacja odrzuciła zapis:\n' + (j.error || ('HTTP ' + r.status));
        msg.className = 'err';
      }
    }, function () {
      msg.textContent = 'Odpowiedź nie jest JSON-em (HTTP ' + r.status + ')';
      msg.className = 'err';
    });
  }, function (e) {
    msg.textContent = 'Błąd sieci: ' + e;
    msg.className = 'err';
  });
}

loadFromEngine(false);
</script>
</body>
</html>
`
