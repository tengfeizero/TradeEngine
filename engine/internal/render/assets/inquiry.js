/* 询盘表单增强：没有这个脚本表单也能提交（原生 POST + 303 跳转），
   有这个脚本则不刷新页面。刻意保持零依赖、无构建步骤。 */
(function () {
  var form = document.querySelector('form[data-inquiry]');
  if (!form) return;

  var byName = function (key) {
    return form.querySelector('[data-inq="' + key + '"]');
  };
  var setValue = function (key, value) {
    var el = byName(key);
    if (el) el.value = value;
  };

  // 时间戳：服务端用它判断「3 秒内提交」的机器人
  setValue('render_ts', String(Date.now()));
  // 来源页与 referrer：决定后续能不能做渠道归因，比询盘本身还重要
  setValue('source_page', location.pathname + location.search);
  setValue('referrer', document.referrer || '');

  var params = new URLSearchParams(location.search);
  ['utm_source', 'utm_medium', 'utm_campaign'].forEach(function (key) {
    setValue(key, params.get(key) || '');
  });

  // 目录下载的留资钩子（M5-9）：页脚下载链接带 ?intent=catalog 指向本页时，
  // 预填一句可编辑的留言——不隐藏、不强制，客户想改随时改。
  if (params.get('intent') === 'catalog') {
    var box = byName('message');
    if (box && !box.value) {
      box.value = (form.dataset.catalogIntent || 'Please send me the full product catalogue and price list.') + '\n';
    }
  }

  var note = form.querySelector('.form-note');
  var button = form.querySelector('button[type="submit"]');
  var originalLabel = button ? button.textContent : '';

  function show(message, kind) {
    if (!note) return;
    note.textContent = message;
    note.className = 'form-note ' + kind;
    note.hidden = false;
  }

  form.addEventListener('submit', function (event) {
    event.preventDefault();
    if (button) {
      button.disabled = true;
      button.textContent = form.dataset.sending || originalLabel;
    }
    if (note) note.hidden = true;

    // 组装 JSON：表单里的 hidden 字段直接带上，UTM 收成一个对象
    var payload = {};
    new FormData(form).forEach(function (value, key) {
      if (key === '_hp' && value === '') return;
      if (key === 'cf-turnstile-response') {
        payload.turnstile_token = value;
        return;
      }
      payload[key] = value;
    });
    payload.utm = {
      source: payload.utm_source || '',
      medium: payload.utm_medium || '',
      campaign: payload.utm_campaign || ''
    };
    delete payload.utm_source;
    delete payload.utm_medium;
    delete payload.utm_campaign;

    fetch(form.action, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      credentials: 'omit'
    })
      .then(function (response) {
        return response
          .json()
          .catch(function () {
            return {};
          })
          .then(function (body) {
            if (response.ok) {
              form.reset();
              setValue('render_ts', String(Date.now()));
              show(form.dataset.thanks || 'OK', 'ok');
              return;
            }
            // 字段级错误能直接指导客户改哪一项，比一句「提交失败」有用得多
            var message = form.dataset.error || 'Error';
            if (body && body.error) {
              message = body.field ? body.error + ' — ' + body.field : body.error;
              if (body.error === 'rate_limited') message = form.dataset.ratelimited || message;
            }
            show(message, 'err');
          });
      })
      .catch(function () {
        show(form.dataset.error || 'Error', 'err');
      })
      .then(function () {
        if (button) {
          button.disabled = false;
          button.textContent = originalLabel;
        }
      });
  });
})();
