package webchat

import (
	"html/template"
	"net/http"
)

// pageTpl renders the single-file chat UI. It is intentionally dependency-free
// so the gateway serves it even on minimal installs.
var pageTpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Clawy WebChat</title>
<style>
  :root { color-scheme: dark; }
  body { margin:0; font-family:system-ui,sans-serif; background:#101418; color:#e6e6e6;
         display:flex; flex-direction:column; height:100vh; }
  header { padding:.75rem 1rem; background:#161b22; font-weight:600; border-bottom:1px solid #232a33; }
  #log { flex:1; overflow-y:auto; padding:1rem; display:flex; flex-direction:column; gap:.5rem; }
  .msg { max-width:80%; padding:.5rem .75rem; border-radius:.75rem; white-space:pre-wrap; word-break:break-word; }
  .user { align-self:flex-end; background:#1f4fd8; }
  .bot  { align-self:flex-start; background:#1d242e; }
  form { display:flex; gap:.5rem; padding:.75rem; border-top:1px solid #232a33; background:#161b22; }
  input[type=text] { flex:1; background:#0d1117; color:#e6e6e6; border:1px solid #2c3440;
                     border-radius:.5rem; padding:.5rem .75rem; }
  button { background:#1f4fd8; border:none; color:#fff; border-radius:.5rem; padding:.5rem 1rem; cursor:pointer; }
  button:disabled { opacity:.5; cursor:default; }
  #status { font-size:.75rem; color:#8aa; padding:0 1rem .5rem; }
</style>
</head>
<body>
<header>🐾 Clawy</header>
<div id="log"></div>
<div id="status">connecting…</div>
<form id="f"><input type="text" id="m" autocomplete="off" placeholder="Message Clawy…"><button>Send</button></form>
<script>
(function () {
  var log = document.getElementById('log'), st = document.getElementById('status');
  var form = document.getElementById('f'), input = document.getElementById('m');

  var id = localStorage.getItem('clawy_webchat_id');
  if (!id) { id = crypto.randomUUID(); localStorage.setItem('clawy_webchat_id', id); }

  var qs = new URLSearchParams(location.search);
  var token = localStorage.getItem('clawy_webchat_token') || qs.get('token') || '';
  if (qs.get('token')) { token = qs.get('token'); localStorage.setItem('clawy_webchat_token', token);
                         qs.delete('token'); history.replaceState(null,'',qs.toString()?location.pathname+'?'+qs:location.pathname); }

  function add(cls, text) {
    var d = document.createElement('div'); d.className = 'msg ' + cls; d.textContent = text;
    log.appendChild(d); log.scrollTop = log.scrollHeight;
  }

  var es = new EventSource('events?id=' + encodeURIComponent(id) + '&token=' + encodeURIComponent(token));
  es.onopen  = function () { st.textContent = 'connected'; };
  es.onerror = function () { st.textContent = 'reconnecting…'; };
  es.onmessage = function (e) {
    try { add('bot', JSON.parse(e.data).content || ''); } catch (_) {}
  };

  form.addEventListener('submit', function (ev) {
    ev.preventDefault();
    var text = input.value.trim(); if (!text) return;
    add('user', text); input.value = '';
    fetch('send', { method:'POST',
      headers: {'Content-Type':'application/json','X-Clawy-Token':token},
      body: JSON.stringify({ id:id, message:text }) })
      .then(function (r) { if (!r.ok) { throw new Error('HTTP '+r.status); } })
      .catch(function (err) { add('bot', '⚠ failed to send ('+err.message+')'); });
  });
})();
</script>
</body>
</html>
`))

// servePage writes the chat UI.
func (c *WebChatChannel) servePage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTpl.Execute(w, nil); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
