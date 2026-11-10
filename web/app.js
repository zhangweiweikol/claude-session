(function () {
  'use strict';

  var initialHash = location.hash;

  var $ = function (sel, root) { return (root || document).querySelector(sel); };
  var $$ = function (sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); };

  var ESC = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };
  function esc(v) {
    return String(v == null ? '' : v).replace(/[&<>"']/g, function (c) { return ESC[c]; });
  }

  var state = {
    projects: [],
    projectId: null,
    folders: [],
    folderFilter: 'unfiled',
    sessions: [],
    sessionId: null,
    detail: null,
    query: '',
    expanded: {},
    expandedInit: false,
    selectMode: false,
    selected: {},
    projectQuery: '',
    folderQuery: '',
    projectsCollapsed: false,
    foldersCollapsed: false,
    sidebarCollapsed: false,
    sortMode: localStorage.getItem('sd.sortMode') || 'recent',
    viewMode: localStorage.getItem('sd.viewMode') === 'tree' ? 'tree' : 'time',
    treeCollapsed: {},
    sessionIndex: {},
    childCount: {},
    crumbExpanded: false,
    aiBusy: {}
  };
  var savedLast = {
    project: Number(localStorage.getItem('sd.lastProject')) || 0,
    folder: localStorage.getItem('sd.lastFolder') || '',
    session: localStorage.getItem('sd.lastSession') || ''
  };
  var listSeq = 0;
  var detailSeq = 0;
  var searchTimer = null;
  var toastTimer = null;

  var FOLDER_ICON = '<svg class="ico-sm folder-ico" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="M2 4.8A1.3 1.3 0 0 1 3.3 3.5h2.4l1.2 1.5h5.8a1.3 1.3 0 0 1 1.3 1.3v5.4a1.3 1.3 0 0 1-1.3 1.3H3.3A1.3 1.3 0 0 1 2 11.7V4.8Z"></path></svg>';
  var ALERT_ICON = '<svg class="ico" viewBox="0 0 16 16"><path d="M8 5.2v3.6"></path><circle cx="8" cy="11.2" r=".4" fill="currentColor" stroke="none"></circle><path d="M6.9 2.6 1.9 11.3A1.3 1.3 0 0 0 3 13.2h10a1.3 1.3 0 0 0 1.1-1.9L9.1 2.6a1.3 1.3 0 0 0-2.2 0Z"></path></svg>';
  var TRASH_ICON = '<svg class="ico-sm folder-ico" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4.5h10M6.2 4.5V3h3.6v1.5M4.4 4.5l.5 8.3h6.2l.5-8.3"></path></svg>';
  var COPY_ICON = '<svg class="ico-sm copy-ico" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="5.6" y="5.6" width="7.8" height="7.8" rx="1.1"></rect><path d="M3.6 10.4H3a1 1 0 0 1-1-1V3.4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v.6"></path></svg>';
  var FOLDER_OPEN_ICON = '<svg class="ico-sm folder-ico" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="M2 4.8A1.3 1.3 0 0 1 3.3 3.5h2.4l1.2 1.5h5.8a1.3 1.3 0 0 1 1.3 1.3v.7H4.7L2 12V4.8Z"></path><path d="M4.7 7h9.4l-1.6 4.7a1.3 1.3 0 0 1-1.2.9H2.6L4.7 7Z"></path></svg>';
  var CHEV_ICON = '<svg class="ico-sm chev" viewBox="0 0 16 16" aria-hidden="true"><path d="M4 6l4 4 4-4"></path></svg>';
  var CHEV_RIGHT_ICON = '<svg class="ico-sm chev" viewBox="0 0 16 16" aria-hidden="true"><path d="M6 4l4 4-4 4"></path></svg>';
  var PLAY_ICON = '<svg class="ico ico-sm" viewBox="0 0 16 16" aria-hidden="true"><path d="M5 3.6v8.8L12 8 5 3.6Z"></path></svg>';
  var RENAME_ICON = '<svg class="ico-sm" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M11.2 2.6l2.2 2.2L6 12.2H3.8v-2.2L11.2 2.6Z"></path></svg>';
  var MOVE_ICON = '<svg class="ico-sm" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h9M9.8 3.5 12.3 6 9.8 8.5M13 10H4M6.2 7.5 3.7 10l2.5 2.5"></path></svg>';
  var BRANCH_ICON = '<svg class="ico-sm" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="4.5" cy="3.5" r="1.5"></circle><circle cx="4.5" cy="12.5" r="1.5"></circle><circle cx="11.5" cy="5.5" r="1.5"></circle><path d="M4.5 5v6M11.5 7v.5a3 3 0 0 1-3 3H4.5"></path></svg>';
  var COMPACT_ICON = '<svg class="ico-sm" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M4 2.6l4 3.6 4-3.6M4 13.4l4-3.6 4 3.6"></path></svg>';
  var FORK_ICON = '<svg class="ico-sm" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="4.5" cy="3.5" r="1.5"></circle><circle cx="11.5" cy="3.5" r="1.5"></circle><circle cx="8" cy="12.5" r="1.5"></circle><path d="M4.5 5v1a2 2 0 0 0 2 2h3a2 2 0 0 0 2-2V5M8 8v3"></path></svg>';

  function api(path, opts) {
    var options = opts || {};
    if (!options.headers) options.headers = {};
    options.headers['Content-Type'] = 'application/json';
    return fetch(path, options).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (data) {
        if (!res.ok) throw new Error(data.error || ('请求失败 (' + res.status + ')'));
        return data;
      });
    });
  }

  function toast(message, kind) {
    var el = $('#toast');
    el.textContent = message;
    el.className = 'toast' + (kind === 'error' ? ' is-error' : '');
    el.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { el.hidden = true; }, kind === 'error' ? 5000 : 2500);
  }

  function setSync(text, isError) {
    $('#syncText').textContent = text;
    $('#syncState').classList.toggle('is-error', !!isError);
  }

  function currentProject() {
    return state.projects.filter(function (p) { return p.id === state.projectId; })[0] || null;
  }

  function folderParam() {
    if (state.folderFilter === 'all') return '';
    if (state.folderFilter === 'unfiled') return 'unfiled';
    if (state.folderFilter === 'categorized') return 'categorized';
    return state.folderFilter.slice(2);
  }

  /* ---------- 项目 ---------- */

  function loadProjects() {
    return api('/api/projects').then(function (data) {
      state.projects = data.projects || [];
      renderProjects();
      if (!state.projectId && state.projects.length) {
        var target = state.projects.filter(function (p) { return p.id === savedLast.project; })[0] || state.projects[0];
        return selectProject(target.id);
      }
      if (state.projectId && !state.projects.some(function (p) { return p.id === state.projectId; })) {
        state.projectId = null;
        clearDetail();
        if (state.projects.length) return selectProject(state.projects[0].id);
      }
    });
  }

  function renderProjects() {
    var root = $('#projectList');
    if (!state.projects.length) {
      root.innerHTML = '<div class="side-empty">没有扫描到项目</div>';
      return;
    }
    var q = state.projectQuery.trim().toLowerCase();
    var visible = state.projects.filter(function (p) {
      if (!q) return true;
      return (p.name + ' ' + (p.original_name || '') + ' ' + p.cwd).toLowerCase().indexOf(q) !== -1;
    });
    if (!visible.length) {
      root.innerHTML = '<div class="project-empty">没有匹配的项目' +
        '<button class="btn btn-sm" data-add-project-empty>添加项目</button></div>';
      return;
    }
    if (q) {
      root.innerHTML = visible.map(projectRow).join('');
      return;
    }

    var pinned = visible.filter(function (p) { return p.pinned; });
    var recent = visible.filter(function (p) { return !p.pinned && p.last_opened_at > 0; })
      .sort(function (a, b) { return b.last_opened_at - a.last_opened_at; })
      .slice(0, 5);
    var recentIds = {};
    recent.forEach(function (p) { recentIds[p.id] = true; });
    var others = visible.filter(function (p) { return !p.pinned && !recentIds[p.id]; });

    var html = '';
    if (pinned.length) {
      html += '<div class="project-group-label">收藏</div>' + pinned.map(projectRow).join('');
    }
    if (recent.length) {
      html += '<div class="project-group-label">最近</div>' + recent.map(projectRow).join('');
    }
    if (others.length) {
      html += '<div class="project-group-label">其他项目</div>' + others.map(projectRow).join('');
    }
    root.innerHTML = html;
  }

  function projectRow(p) {
    var active = p.id === state.projectId;
    var pinnedList = p.pinned ? state.projects.filter(function (x) { return x.pinned; }) : [];
    var orderIdx = pinnedList.indexOf(p);
    var showOrder = p.pinned && !state.projectQuery.trim();
    var orderable = showOrder && orderIdx >= 0 && pinnedList.length > 1;
    var canUp = orderable && orderIdx > 0;
    var canDown = orderable && orderIdx < pinnedList.length - 1;
    var orderActions = showOrder
      ? '<button class="project-act" data-project-up="' + p.id + '" title="上移" aria-label="上移"' + (canUp ? '' : ' disabled') + '>↑</button>' +
        '<button class="project-act" data-project-down="' + p.id + '" title="下移" aria-label="下移"' + (canDown ? '' : ' disabled') + '>↓</button>'
      : '';
    return '<div class="project-row' + (active ? ' is-active' : '') + '">' +
      '<button class="project-item' + (active ? ' is-active' : '') + '" data-project="' + p.id + '">' +
      '<span class="p-main"><span class="p-name">' + esc(p.name) + '</span><span class="p-count">' + p.session_count + '</span></span>' +
      '<span class="p-path">' + esc(p.cwd) + '</span>' +
      '</button>' +
      '<span class="p-actions">' +
      '<button class="project-star' + (p.pinned ? ' is-on' : '') + '" data-pin-project="' + p.id + '"' +
      ' title="' + (p.pinned ? '取消收藏' : '收藏项目') + '"' +
      ' aria-label="' + (p.pinned ? '取消收藏' : '收藏项目') + '"' +
      ' aria-pressed="' + (p.pinned ? 'true' : 'false') + '">' +
      '<svg class="ico-xs" viewBox="0 0 16 16" aria-hidden="true"><path d="M8 2.2l1.7 3.5 3.8.6-2.8 2.7.7 3.8L8 11l-3.4 1.8.7-3.8-2.8-2.7 3.8-.6L8 2.2Z"></path></svg>' +
      '</button>' +
      orderActions +
      '<button class="project-act is-alias" data-alias-project="' + p.id + '" title="设置别名" aria-label="设置别名">' +
      '<svg class="ico-xs" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M11.2 2.6l2.2 2.2L6 12.2H3.8v-2.2L11.2 2.6Z"></path></svg>' +
      '</button>' +
      '<button class="project-act" data-del-project="' + p.id + '" title="删除项目" aria-label="删除项目">×</button>' +
      '</span>' +
      '</div>';
  }

  function setProjectSearch(open) {
    var box = $('#projectSearch');
    box.hidden = !open;
    if (open) {
      $('#projectQuery').focus();
      $('#projectQuery').select();
    } else {
      $('#projectQuery').value = '';
      state.projectQuery = '';
      renderProjects();
    }
  }

  function setFolderSearch(open) {
    var box = $('#folderSearch');
    box.hidden = !open;
    if (open) {
      if (state.foldersCollapsed) {
        state.foldersCollapsed = false;
        applySidebarState();
      }
      $('#folderQuery').focus();
      $('#folderQuery').select();
    } else {
      $('#folderQuery').value = '';
      state.folderQuery = '';
      renderFolders();
    }
  }

  // 每个项目记住上次点击的归类；没有记录时用「已归类」。
  function rememberedFilter(id) {
    var saved = '';
    try { saved = localStorage.getItem('sd.folderFilter.' + id) || ''; } catch (e) { /* 隐私模式 */ }
    if (!validRememberedFilter(saved)) {
      // 旧版只把最后一次点击存在 sd.lastFolder，属于最后打开的项目。
      saved = savedLast.project === id ? savedLast.folder : '';
    }
    return validRememberedFilter(saved) ? saved : 'categorized';
  }

  function rememberFilter(id, filter) {
    if (!id) return;
    try { localStorage.setItem('sd.folderFilter.' + id, filter); } catch (e) { /* 隐私模式 */ }
  }

  function validRememberedFilter(filter) {
    return filter === 'all' || filter === 'unfiled' || filter === 'categorized' || /^f:\d+$/.test(filter);
  }

  function selectProject(id, fromUser) {
    if (state.projectId === id) return Promise.resolve();
    var filter = rememberedFilter(id);
    localStorage.setItem('sd.lastProject', String(id));
    var sidebar = $('.sidebar');
    if (sidebar) sidebar.classList.remove('is-open');
    state.projectId = id;
    var project = state.projects.filter(function (p) { return p.id === id; })[0];
    if (project) {
      project.last_opened_at = Math.floor(Date.now() / 1000);
      api('/api/projects/' + id + '/touch', { method: 'POST' }).catch(function () {});
    }
    state.folderFilter = filter;
    state.sessionId = null;
    state.query = '';
    state.expanded = {};
    state.expandedInit = false;
    resetSelection();
    $('#search').value = '';
    if (fromUser) {
      try { history.replaceState(null, '', location.pathname); } catch (e) { /* 忽略 */ }
    }
    clearDetail();
    renderProjects();
    renderFolderTabs();
    syncAITidyButton();
    return loadFolders().then(function () {
      // 记忆里的归类可能已被删除，回退到「已归类」。
      if (state.folderFilter.indexOf('f:') === 0 && !folderFilterValid(state.folderFilter)) {
        state.folderFilter = 'categorized';
        rememberFilter(state.projectId, 'categorized');
        renderFolders();
        renderFolderTabs();
      }
      return loadSessions();
    });
  }

  /* ---------- 归类 ---------- */

  function loadFolders() {
    if (!state.projectId) return Promise.resolve();
    return api('/api/projects/' + state.projectId + '/folders').then(function (data) {
      state.folders = data.folders || [];
      if (!state.expandedInit) {
        state.expanded = { categorized: true };
        state.folders.filter(function (f) { return !f.parent_id; }).forEach(function (f) {
          state.expanded[f.id] = true;
        });
        state.expandedInit = true;
      }
      renderFolders();
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  function renderFolders() {
    var root = $('#folderTree');
    var q = state.folderQuery.trim().toLowerCase();
    var html = '';

    html += folderStaticItem('all', '全部', '', FOLDER_ICON);
    html += folderStaticItem('unfiled', '待归类', '', FOLDER_ICON);

    if (!q) {
      html += categorizedRow(false);
      if (state.expanded.categorized) {
        state.folders.filter(function (f) { return !f.parent_id; }).forEach(function (f, i) {
          html += renderTree(f, 1, String(i + 1), '', null);
        });
      }
      html += folderStaticItem('trash', '回收站', '', TRASH_ICON);
      root.innerHTML = html;
      return;
    }

    var visible = {};
    state.folders.forEach(function (f) {
      if (f.name.toLowerCase().indexOf(q) === -1) return;
      var cur = f;
      while (cur) {
        visible[cur.id] = true;
        var parentID = cur.parent_id;
        cur = parentID ? state.folders.filter(function (x) { return x.id === parentID; })[0] : null;
      }
    });
    var matchCount = Object.keys(visible).length;

    html += categorizedRow(matchCount > 0);
    if (matchCount) {
      state.folders.filter(function (f) { return !f.parent_id && visible[f.id]; }).forEach(function (f, i) {
        html += renderTree(f, 1, String(i + 1), q, visible);
      });
    } else {
      html += '<div class="folder-empty">没有匹配的归类' +
        '<button class="btn btn-sm" data-clear-folder-search>清除搜索</button></div>';
    }
    html += folderStaticItem('trash', '回收站', '', TRASH_ICON);
    root.innerHTML = html;
  }

  function categorizedRow(forceExpanded) {
    var expanded = forceExpanded || !!state.expanded.categorized;
    var active = state.folderFilter === 'categorized';
    return '<div class="folder-row' + (active ? ' is-active' : '') + '" style="padding-left:0">' +
      '<button class="folder-item' + (active ? ' is-active' : '') + '" data-toggle-categorized="1">' +
      chevIcon(expanded, true) + (expanded ? FOLDER_OPEN_ICON : FOLDER_ICON) +
      '<span class="f-name">已归类</span><span></span></button>' +
      '<span class="folder-actions">' +
      '<button class="folder-act" data-add-folder="0" title="新建归类" aria-label="新建归类">+</button>' +
      '</span></div>';
  }

  function renderTree(folder, depth, number, query, visible) {
    var active = state.folderFilter === 'f:' + folder.id;
    var siblings = state.folders.filter(function (f) { return (f.parent_id || 0) === (folder.parent_id || 0); });
    var orderIdx = siblings.indexOf(folder);
    var orderable = !query && orderIdx >= 0 && siblings.length > 1;
    var canUp = orderable && orderIdx > 0;
    var canDown = orderable && orderIdx < siblings.length - 1;
    var kids = state.folders.filter(function (f) { return f.parent_id === folder.id; });
    var visibleKids = query ? kids.filter(function (k) { return visible[k.id]; }) : kids;
    var maxDepth = 4;
    var canExpand = query ? visibleKids.length > 0 : (kids.length > 0 && depth < maxDepth);
    var expanded = query ? visibleKids.length > 0 : (canExpand && !!state.expanded[folder.id]);
    var nameHtml = query ? highlightText(folder.name, query) : esc(folder.name);
    var html = '<div class="folder-row' + (active ? ' is-active' : '') + '" style="padding-left:' + (8 + (depth - 1) * 14) + 'px">' +
      '<button class="folder-item' + (active ? ' is-active' : '') + '" data-filter="f:' + folder.id + '"' +
      ' data-folder-id="' + folder.id + '" data-has-kids="' + (canExpand ? '1' : '0') + '">' +
      chevIcon(expanded, canExpand) +
      (expanded ? FOLDER_OPEN_ICON : FOLDER_ICON) +
      '<span class="f-no">' + number + '</span>' +
      '<span class="f-name">' + nameHtml +
      (folder.source === 'ai' ? '<span class="f-ai" title="AI 创建的归类">AI</span>' : '') + '</span>' +
      (!query && kids.length > 0 && depth >= maxDepth ? '<span class="f-more">…</span>' : '') +
      '<span></span></button>' +
      '<span class="folder-actions">' +
      '<button class="folder-act" data-folder-resume="' + folder.id + '" title="恢复最后一次会话" aria-label="恢复最后一次会话">' + PLAY_ICON + '</button>' +
      '<button class="folder-act" data-folder-up="' + folder.id + '" title="上移" aria-label="上移"' + (canUp ? '' : ' disabled') + '>↑</button>' +
      '<button class="folder-act" data-folder-down="' + folder.id + '" title="下移" aria-label="下移"' + (canDown ? '' : ' disabled') + '>↓</button>' +
      '<button class="folder-act" data-add-folder="' + folder.id + '" title="新建子归类" aria-label="新建子归类">+</button>' +
      '<button class="folder-act" data-rename-folder="' + folder.id + '" title="重命名归类" aria-label="重命名归类">' +
      '<svg class="ico-xs" viewBox="0 0 16 16" aria-hidden="true"><path d="M11.2 2.6l2.2 2.2L6 12.2H3.8v-2.2L11.2 2.6Z"></path></svg>' +
      '</button>' +
      '<button class="folder-act danger" data-del-folder="' + folder.id + '" title="删除归类" aria-label="删除归类">×</button>' +
      '</span></div>';
    if (expanded) {
      kids.forEach(function (k, i) {
        if (query && !visible[k.id]) return;
        html += renderTree(k, depth + 1, number + '.' + (i + 1), query, visible);
      });
    }
    return html;
  }

  function highlightText(text, query) {
    var safe = esc(text);
    if (!query) return safe;
    var pattern = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    try {
      return safe.replace(new RegExp('(' + pattern + ')', 'ig'), '<mark>$1</mark>');
    } catch (e) {
      return safe;
    }
  }

  function chevIcon(expanded, hasKids) {
    return '<svg class="ico-sm chev' + (expanded ? '' : ' is-collapsed') + (hasKids ? '' : ' is-placeholder') +
      '" viewBox="0 0 16 16" aria-hidden="true"><path d="M4 6l4 4 4-4"></path></svg>';
  }

  function folderStaticItem(filter, name, count, icon) {
    return '<button class="folder-item' + (state.folderFilter === filter ? ' is-active' : '') + '" data-filter="' + filter + '">' +
      chevIcon(false, false) + icon +
      '<span class="f-name">' + esc(name) + '</span><span>' + (count === '' ? '' : count) + '</span></button>';
  }

  function renderFolderTabs() {
    renderCrumb();
  }

  function renderCrumb() {
    var root = $('#listCrumb');
    if (!root) return;
    var filter = state.folderFilter;
    if (filter === 'all') {
      root.innerHTML = '<span class="crumb-root">全部会话</span>';
      return;
    }
    if (filter === 'unfiled') {
      root.innerHTML = '<span class="crumb-root">待归类</span>';
      return;
    }
    if (filter === 'categorized') {
      root.innerHTML = '<span class="crumb-root">已归类</span>';
      return;
    }
    if (filter === 'trash') {
      root.innerHTML = '<span class="crumb-root">回收站</span>';
      return;
    }
    if (filter.indexOf('f:') !== 0) {
      root.innerHTML = '';
      return;
    }
    var id = Number(filter.slice(2));
    var chain = [];
    var cur = state.folders.filter(function (f) { return f.id === id; })[0];
    while (cur) {
      chain.unshift(cur);
      var parentID = cur.parent_id;
      cur = parentID ? state.folders.filter(function (f) { return f.id === parentID; })[0] : null;
    }
    // 层级超过 3 级时折叠中间层，保留首级与末级，点击 … 展开。
    var visible = state.crumbExpanded || chain.length <= 3
      ? chain
      : [chain[0], null, chain[chain.length - 1]];
    var html = '<button class="crumb-link" data-crumb="categorized">已归类</button>';
    visible.forEach(function (f) {
      html += '<span class="crumb-sep">/</span>';
      if (f === null) {
        html += '<button class="crumb-more" data-crumb-more="1" title="展开中间层级">…</button>';
      } else if (f === chain[chain.length - 1]) {
        html += '<span class="crumb-current">' + esc(f.name) + '</span>';
      } else {
        html += '<button class="crumb-link" data-crumb="f:' + f.id + '">' + esc(f.name) + '</button>';
      }
    });
    root.innerHTML = html;
  }

  function sortSessions(list) {
    var arr = list.slice();
    if (state.sortMode === 'messages') {
      arr.sort(function (a, b) {
        return (b.message_count - a.message_count) ||
          String(b.last_ts || '').localeCompare(String(a.last_ts || ''));
      });
    } else if (state.sortMode === 'title') {
      arr.sort(function (a, b) {
        return String(a.title || '').localeCompare(String(b.title || ''), 'zh-CN');
      });
    } else {
      arr.sort(function (a, b) {
        return String(b.last_ts || '').localeCompare(String(a.last_ts || ''));
      });
    }
    return arr;
  }

  function animateTree(el) {
    if (!el) return;
    el.classList.remove('is-animating');
    void el.offsetWidth;
    el.classList.add('is-animating');
    clearTimeout(el._animT);
    el._animT = setTimeout(function () { el.classList.remove('is-animating'); }, 220);
  }

  function setFolderFilter(filter) {
    state.folderFilter = filter;
    localStorage.setItem('sd.lastFolder', filter);
    // 回收站不作为项目默认归类，只记真实的归类选择。
    if (filter !== 'trash') rememberFilter(state.projectId, filter);
    state.crumbExpanded = false;
    state.sessionId = null;
    initialHash = null;
    resetSelection();
    clearDetail();
    renderFolders();
    renderFolderTabs();
    updateViewSwitch();
    loadSessions();
  }

  function addFolder(parentID) {
    if (!state.projectId) {
      toast('请先选择项目', 'error');
      return;
    }
    folderDialog(parentID || 0).then(function (result) {
      if (!result) return;
      if (!result.name) {
        toast('归类名称不能为空', 'error');
        return;
      }
      api('/api/projects/' + state.projectId + '/folders', {
        method: 'POST',
        body: JSON.stringify({ name: result.name, parent_id: result.parent_id || null })
      }).then(function () {
        state.expanded.categorized = true;
        toast('归类已创建');
        return loadFolders();
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function folderSelectOptions(selectedID) {
    var html = '<option value="0"' + (!selectedID ? ' selected' : '') + '>已归类（顶层）</option>';
    function walk(parentID, depth) {
      state.folders.filter(function (f) { return (f.parent_id || 0) === parentID; }).forEach(function (f) {
        html += '<option value="' + f.id + '"' + (f.id === selectedID ? ' selected' : '') + '>' +
          '　'.repeat(depth) + esc(f.name) + '</option>';
        walk(f.id, depth + 1);
      });
    }
    walk(0, 0);
    return html;
  }

  function folderDialog(defaultParentID) {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>新建归类</h3>' +
        '<p>归类支持多层嵌套，只存在于CC会话管理中，不影响 Claude Code 目录。</p></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body">' +
        '<div class="field"><label for="fdName">归类名称</label>' +
        '<input class="text-input" id="fdName" autocomplete="off" placeholder="例如：接口文档"></div>' +
        '<div class="field"><label for="fdParent">父归类</label>' +
        '<select id="fdParent">' + folderSelectOptions(defaultParentID) + '</select></div>' +
        '</div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn btn-primary" data-ok>创建</button></footer>' +
        '</div></div>';

      var input = root.querySelector('#fdName');
      var parentSel = root.querySelector('#fdParent');
      var backdrop = root.querySelector('.modal-backdrop');
      var done = false;

      function finish(value) {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(value);
      }
      function submit() {
        finish({ name: input.value.trim(), parent_id: Number(parentSel.value) || 0 });
      }

      root.querySelector('[data-ok]').addEventListener('click', submit);
      root.querySelector('[data-cancel]').addEventListener('click', function () { finish(null); });
      root.querySelector('[data-close]').addEventListener('click', function () { finish(null); });
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(null); });
      input.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); submit(); }
        if (e.key === 'Escape') { e.preventDefault(); finish(null); }
      });
      input.focus();
    });
  }

  function folderPickerDialog(title, message, selectedID) {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>' + esc(title) + '</h3>' +
        (message ? '<p>' + esc(message) + '</p>' : '') + '</div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body"><div class="field"><label for="fpFolder">归类</label>' +
        '<select id="fpFolder">' + folderOptions(state.folders, selectedID || 0) + '</select></div></div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn btn-primary" data-ok>确定</button></footer>' +
        '</div></div>';

      var select = root.querySelector('#fpFolder');
      var backdrop = root.querySelector('.modal-backdrop');
      var done = false;

      function finish(value) {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(value);
      }

      root.querySelector('[data-ok]').addEventListener('click', function () {
        finish(Number(select.value) || 0);
      });
      root.querySelector('[data-cancel]').addEventListener('click', function () { finish(null); });
      root.querySelector('[data-close]').addEventListener('click', function () { finish(null); });
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(null); });
      select.focus();
    });
  }

  function renameFolder(folderID) {
    var folder = state.folders.filter(function (f) { return f.id === folderID; })[0];
    if (!folder) return;
    promptDialog({
      title: '重命名归类',
      label: '归类名称',
      value: folder.name
    }).then(function (name) {
      if (name === null) return;
      if (!name) {
        toast('归类名称不能为空', 'error');
        return;
      }
      api('/api/projects/' + state.projectId + '/folders/' + folderID, {
        method: 'PATCH',
        body: JSON.stringify({ name: name })
      }).then(function () {
        toast('归类已重命名');
        return loadFolders().then(function () { updatePaneTitle(); });
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function resumeFolderLatest(folderID) {
    if (!state.projectId) return;
    api('/api/sessions?project_id=' + state.projectId + '&folder_id=' + folderID + '&limit=1')
      .then(function (data) {
        var latest = (data.sessions || [])[0];
        if (!latest) {
          toast('这个归类还没有会话', 'error');
          return;
        }
        return openSessionById(latest.id);
      })
      .catch(function (e) {
        toast(e.message, 'error');
      });
  }

  function moveFolder(folderID, direction) {
    if (!state.projectId) return;
    api('/api/projects/' + state.projectId + '/folders/' + folderID + '/move', {
      method: 'POST',
      body: JSON.stringify({ direction: direction })
    }).then(function () {
      return loadFolders();
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  function deleteFolder(folderID) {
    var folder = state.folders.filter(function (f) { return f.id === folderID; })[0];
    if (!folder) return;
    confirmDialog({
      title: '删除归类',
      message: '「' + folder.name + '」及其子归类会被删除，其中的会话会移到「待归类」。',
      confirmText: '删除归类',
      danger: true
    }).then(function (ok) {
      if (!ok) return;
      api('/api/projects/' + state.projectId + '/folders/' + folderID, { method: 'DELETE' }).then(function () {
        toast('归类已删除');
        return loadFolders().then(function () {
          if (state.folderFilter.indexOf('f:') === 0) {
            var fid = Number(state.folderFilter.slice(2));
            if (!state.folders.some(function (f) { return f.id === fid; })) {
              state.folderFilter = 'all';
              rememberFilter(state.projectId, 'all');
            }
          }
          renderFolders();
          renderFolderTabs();
          return loadSessions(true);
        });
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  /* ---------- 会话列表 ---------- */

  function loadSessions(silent) {
    if (!state.projectId) {
      renderListState('empty');
      return Promise.resolve();
    }
    var seq = ++listSeq;
    if (!silent) renderListState('loading');
    var params = new URLSearchParams({ project_id: state.projectId });
    if (state.folderFilter === 'trash') {
      params.set('deleted', '1');
    } else {
      var fp = folderParam();
      if (fp) params.set('folder_id', fp);
    }
    if (state.query) params.set('q', state.query);

    return api('/api/sessions?' + params.toString()).then(function (data) {
      if (seq !== listSeq) return;
      state.sessions = sortSessions(data.sessions || []);
      renderSessions();
      if (state.query && !state.sessions.length) {
        state.sessionId = null;
        clearDetail('search');
      }
      if (!state.sessionId && !initialHash && !state.query && state.sessions.length) {
        selectSession(state.sessions[0].id);
      }
    }).catch(function (e) {
      if (seq !== listSeq) return;
      renderListState('error', e.message);
    });
  }

  function updatePaneTitle() {
    var p = currentProject();
    var root = $('#listSub');
    var crumb = $('#listCrumb');
    var count = $('#listCount');
    if (p) {
      var path = p.cwd || '';
      var label = p.name || path.split(/[\\/]/).filter(Boolean).pop() || path;
      root.innerHTML =
        '<span class="project">' +
        '<button class="project-name" id="projectName" data-path="' + esc(path) + '" aria-label="复制项目路径：' + esc(path) + '">' + esc(label) + '</button>' +
        '<span class="copy-mark" aria-hidden="true">' + COPY_ICON + '</span>' +
        '<span class="tip" role="tooltip">' + esc(path) + '<span class="tip-key">点击复制</span></span>' +
        '</span>';
      if (crumb) crumb.hidden = false;
      if (count) {
        count.textContent = state.query ? '搜索“' + state.query + '”' : state.sessions.length + ' 个会话';
      }
    } else {
      root.innerHTML = '<span class="pane-empty">选择左侧项目</span>';
      if (crumb) { crumb.innerHTML = ''; crumb.hidden = true; }
      if (count) count.textContent = '';
    }
    var emptyBtn = $('#btnEmptyTrash');
    if (emptyBtn) emptyBtn.hidden = state.folderFilter !== 'trash';
  }

  function copyProjectPath(btn) {
    var path = btn.dataset.path || '';
    if (!path) return;
    var done = function () {
      toast('项目路径已复制');
      var project = btn.closest('.project');
      if (!project) return;
      var old = project.querySelector('.copied');
      if (old) old.remove();
      var mark = document.createElement('span');
      mark.className = 'copied';
      mark.textContent = '已复制';
      btn.after(mark);
      setTimeout(function () { mark.remove(); }, 1500);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(path).then(done).catch(function () {
        toast('复制失败，请手动选择文本', 'error');
      });
    } else {
      toast('当前浏览器不支持剪贴板', 'error');
    }
  }

  function sessionStatus(s) {
    if (s.active) return { cls: 'is-live', label: '进行中' };
    if (s.state === 'deleted') return { cls: 'is-deleted', label: '已删除' };
    if (isRecent(s.last_ts)) return { cls: 'is-recent', label: '最近' };
    return { cls: '', label: '' };
  }

  function shortPath(p) {
    var parts = String(p || '').split(/[\\/]/).filter(Boolean);
    if (parts.length <= 2) return p || '';
    return '…\\' + parts.slice(-2).join('\\');
  }

  function fmtTokens(n) {
    if (n >= 1000000) return (n / 1000000).toFixed(n >= 10000000 ? 0 : 1) + 'M';
    if (n >= 1000) return (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k';
    return String(n);
  }

  function sessionGroupLabel(ts) {
    if (!ts) return '更早';
    var d = new Date(ts);
    if (isNaN(d.getTime())) return '更早';
    var now = new Date();
    var today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    var day = new Date(d.getFullYear(), d.getMonth(), d.getDate());
    var days = Math.round((today - day) / 86400000);
    if (days <= 0) return '今天';
    if (days === 1) return '昨天';
    if (days <= 7) return '最近 7 天';
    if (days <= 30) return '最近 30 天';
    return '更早';
  }

  function treeAllowed() {
    return state.folderFilter !== 'trash';
  }

  // 时间视图的关联 chip：父会话、派生计数；谱系树里不显示派生计数。
  function relChipHtml(s) {
    if (!s.parent_id) return '';
    var parentInList = !!state.sessionIndex[s.parent_id];
    var title = s.parent_title ? '「' + esc(s.parent_title) + '」' : '原会话';
    var note = '';
    if (s.parent_state === 'deleted') note = '<span class="rel-note">在回收站</span>';
    else if (s.parent_state === 'missing') note = '<span class="rel-note">已清理</span>';
    else if (!s.parent_title) note = '<span class="rel-note">已删除</span>';
    var attrs = parentInList
      ? ' role="button" tabindex="0" data-goto-session="' + esc(s.parent_id) + '" title="跳转到父会话"'
      : ' title="父会话不在当前筛选结果中"';
    return '<span class="rel-chip' + (parentInList ? ' is-link' : '') + '"' + attrs + '>' +
      '<span class="rel-ico">' + BRANCH_ICON + '</span>' +
      '<span>派生自</span><span class="rel-name">' + title + '</span>' + note + '</span>';
  }

  function countChipHtml(s) {
    var n = state.childCount[s.id] || 0;
    if (!n || !treeAllowed()) return '';
    return '<span class="rel-chip is-count" role="button" tabindex="0" data-family="' + esc(s.id) + '" title="在谱系视图中查看这一族">' +
      '<span class="rel-ico">' + BRANCH_ICON + '</span><span>' + n + ' 个派生</span></span>';
  }

  function sessionActionsHtml(s) {
    var deleted = s.state === 'deleted';
    if (deleted || state.selectMode || state.folderFilter === 'trash') return '';
    var sid = esc(s.id);
    return '<span class="s-actions">' +
      '<span class="s-act" role="button" tabindex="-1" data-session-open="' + sid + '" title="恢复会话" aria-label="恢复会话">' + PLAY_ICON + '</span>' +
      '<span class="s-act" role="button" tabindex="-1" data-session-rename="' + sid + '" title="重命名" aria-label="重命名">' + RENAME_ICON + '</span>' +
      '<span class="s-act" role="button" tabindex="-1" data-session-compact="' + sid + '" title="压缩上下文（compact）" aria-label="压缩上下文">' + COMPACT_ICON + '</span>' +
      '<span class="s-act" role="button" tabindex="-1" data-session-branch="' + sid + '" title="创建分支（branch）" aria-label="创建分支">' + BRANCH_ICON + '</span>' +
      '<span class="s-act" role="button" tabindex="-1" data-session-fork="' + sid + '" title="复制为副本（fork）" aria-label="复制为副本">' + FORK_ICON + '</span>' +
      '<span class="s-act" role="button" tabindex="-1" data-session-project="' + sid + '" title="归属项目" aria-label="归属项目">' + MOVE_ICON + '</span>' +
      '<span class="s-act" role="button" tabindex="-1" data-move-folder="' + sid + '" title="加入归类" aria-label="加入归类">' + FOLDER_ICON + '</span>' +
      '<span class="s-act danger" role="button" tabindex="-1" data-session-delete="' + sid + '" title="删除" aria-label="删除">' + TRASH_ICON + '</span>' +
      '</span>';
  }

  function sessionMetaHtml(s) {
    var meta = [];
    var status = sessionStatus(s);
    if (status.label) meta.push('<span class="s-state">' + status.label + '</span>');
    if (s.cwd) meta.push('<span class="s-path" title="' + esc(s.cwd) + '">' + esc(shortPath(s.cwd)) + '</span>');
    meta.push('<span>' + s.message_count + ' 条</span>');
    var tokens = (s.tokens_input || 0) + (s.tokens_output || 0);
    if (tokens > 0) meta.push('<span class="s-tok">' + fmtTokens(tokens) + ' tok</span>');
    if (s.git_branch && s.git_branch !== 'HEAD') meta.push('<span class="branch">' + esc(s.git_branch) + '</span>');
    return meta;
  }

  // 概要优先展示 AI 摘要，未生成时回退到全文检索片段或最后一条 prompt。
  function sessionExcerpt(s, deleted) {
    // 搜索命中正文或概要时，优先展示带高亮的片段。
    if (state.query && s.snippet) return highlight(s.snippet);
    if (s.ai_summary) {
      return '<span class="s-ai-tag">AI</span>' + esc(s.ai_summary);
    }
    if (s.snippet) return highlight(s.snippet);
    return esc(s.last_prompt || (deleted ? '（已移到回收站，还原后可用）' : '（没有可展示的 prompt 摘要）'));
  }

  // 建议标题只在会话没有自定义标题时出现，采用后走重命名接口。
  function suggestTitleHtml(s) {
    if (!s.ai_can_apply_title || !s.ai_suggested_title) return '';
    return '<span class="s-title-suggest">' +
      '<span class="s-title-suggest-label">建议标题</span>' +
      '<span class="s-title-suggest-value">' + esc(s.ai_suggested_title) + '</span>' +
      '<span class="s-title-suggest-apply" role="button" tabindex="-1" ' +
      'data-apply-title="' + esc(s.id) + '" data-title="' + esc(s.ai_suggested_title) + '">采用</span>' +
      '</span>';
  }

  function buildSessionItem(s) {
    var deleted = s.state === 'deleted';
    var status = sessionStatus(s);
    var checked = !!state.selected[s.id];
    var cls = 'session-item ' + status.cls +
      (state.selectMode ? ' is-selectable' : '') +
      (checked ? ' is-selected' : '') +
      (s.id === state.sessionId ? ' is-active' : '');
    var excerpt = sessionExcerpt(s, deleted);
    var tp = listTimeParts(deleted && s.deleted_at
      ? new Date(s.deleted_at * 1000).toISOString()
      : s.last_ts);
    var time = tp.date ? tp.date + ' ' + tp.time : '';
    var indexCell = state.selectMode
      ? '<span class="s-index is-check"><span class="s-check' + (checked ? ' is-checked' : '') +
        '" role="checkbox" aria-checked="' + (checked ? 'true' : 'false') + '"></span></span>'
      : '<span class="s-index"><span class="s-date">' + tp.date + '</span><span class="s-clock">' + tp.time + '</span></span>';
    var rowTime = state.selectMode ? '<span class="s-time">' + time + '</span>' : '';
    var meta = sessionMetaHtml(s);
    if (s.parent_id) meta.push(relChipHtml(s));
    var count = countChipHtml(s);
    if (count) meta.push(count);
    var mark = s.parent_id
      ? '<span class="rel-glyph' + (s.active ? ' is-live' : '') + '">' + BRANCH_ICON + '</span>'
      : '<span class="s-dot"></span>';
    return '<button class="' + cls + '" data-session="' + esc(s.id) + '">' +
      indexCell +
      '<span class="s-main">' +
      '<span class="s-title-row">' + mark +
      '<span class="s-title">' + esc(s.title) + '</span>' + rowTime + '</span>' +
      '<span class="s-excerpt">' + excerpt + '</span>' +
      suggestTitleHtml(s) +
      '<span class="s-meta">' + meta.join('') + '</span>' +
      '</span>' + sessionActionsHtml(s) + '</button>';
  }

  // 谱系树：父会话在上、派生会话缩进在下，族内顺序不随排序偏好改变。
  function buildTreeItem(s, childrenMap, opts) {
    opts = opts || {};
    var kids = childrenMap[s.id] || [];
    var collapsed = !!state.treeCollapsed[s.id];
    var deleted = s.state === 'deleted';
    var status = sessionStatus(s);
    var checked = !!state.selected[s.id];
    var cls = 'session-item tree-row ' + status.cls +
      (state.selectMode ? ' is-selectable' : '') +
      (checked ? ' is-selected' : '') +
      (s.id === state.sessionId ? ' is-active' : '');
    var excerpt = sessionExcerpt(s, deleted);
    var tp = listTimeParts(s.last_ts);
    var time = tp.date ? tp.date + ' ' + tp.time : '';
    var meta = [];
    if (opts.stats) meta.push('<span class="tree-stat">' + opts.stats + '</span>');
    meta = meta.concat(sessionMetaHtml(s));
    // 父会话不在当前筛选结果里时，树中作为根显示它的来源。
    if (s.parent_id && !state.sessionIndex[s.parent_id]) meta.push(relChipHtml(s));
    var toggle;
    if (state.selectMode) {
      toggle = '<span class="tree-toggle"><span class="s-check' + (checked ? ' is-checked' : '') +
        '" role="checkbox" aria-checked="' + (checked ? 'true' : 'false') + '"></span></span>';
    } else if (kids.length) {
      toggle = '<span class="tree-toggle" role="button" tabindex="-1" data-tree-toggle="' + esc(s.id) + '" title="' +
        (collapsed ? '展开派生会话' : '收起派生会话') + '" aria-label="展开或收起">' + CHEV_ICON + '</span>';
    } else {
      toggle = '<span class="tree-leaf"></span>';
    }
    var mark = s.parent_id
      ? '<span class="rel-glyph' + (s.active ? ' is-live' : '') + '">' + BRANCH_ICON + '</span>'
      : '';
    var html = '<div class="tree-node' + (collapsed ? ' is-collapsed' : '') + '">' +
      '<button class="' + cls + '" data-session="' + esc(s.id) + '">' +
      toggle +
      '<span class="s-main">' +
      '<span class="s-title-row">' + mark +
      '<span class="s-title">' + esc(s.title) + '</span>' +
      (state.selectMode ? '' : '<span class="s-time">' + time + '</span>') + '</span>' +
      '<span class="s-excerpt">' + excerpt + '</span>' +
      '<span class="s-meta">' + meta.join('') + '</span>' +
      '</span>' + sessionActionsHtml(s) + '</button>' +
      (kids.length
        ? '<div class="tree-children">' +
          kids.map(function (k) { return buildTreeItem(k, childrenMap); }).join('') +
          '</div>'
        : '') +
      '</div>';
    return html;
  }

  function renderTreeSessions() {
    var byId = state.sessionIndex;
    var childrenMap = {};
    var roots = [];
    state.sessions.forEach(function (s) {
      if (s.parent_id && byId[s.parent_id]) {
        (childrenMap[s.parent_id] = childrenMap[s.parent_id] || []).push(s);
      } else {
        roots.push(s);
      }
    });
    Object.keys(childrenMap).forEach(function (id) {
      childrenMap[id].sort(function (a, b) {
        return String(a.last_ts || '').localeCompare(String(b.last_ts || ''));
      });
    });

    function descendantsOf(id) {
      var n = 0;
      (childrenMap[id] || []).forEach(function (c) { n += 1 + descendantsOf(c.id); });
      return n;
    }
    function depthOf(id) {
      var d = 0;
      (childrenMap[id] || []).forEach(function (c) { d = Math.max(d, 1 + depthOf(c.id)); });
      return d;
    }
    function maxTs(id) {
      var best = byId[id] ? byId[id].last_ts : '';
      (childrenMap[id] || []).forEach(function (c) {
        var t = maxTs(c.id);
        if (t > best) best = t;
      });
      return best;
    }
    function sumMessages(id) {
      var n = byId[id] ? byId[id].message_count : 0;
      (childrenMap[id] || []).forEach(function (c) { n += sumMessages(c.id); });
      return n;
    }

    roots.sort(function (a, b) {
      if (state.sortMode === 'title') return String(a.title).localeCompare(String(b.title), 'zh-CN');
      if (state.sortMode === 'messages') return sumMessages(b.id) - sumMessages(a.id);
      return String(maxTs(b.id)).localeCompare(String(maxTs(a.id)));
    });

    return roots.map(function (root) {
      var total = descendantsOf(root.id);
      var depth = depthOf(root.id);
      var label = root.parent_id
        ? '父会话不在筛选'
        : (total ? total + ' 个派生' : '独立会话');
      var stats = label + (depth > 0 ? ' · ' + (depth + 1) + ' 代' : '');
      return buildTreeItem(root, childrenMap, { stats: stats });
    }).join('');
  }

  function renderSessions() {
    updatePaneTitle();
    var root = $('#listBody');
    state.sessionIndex = {};
    state.childCount = {};
    state.sessions.forEach(function (s) { state.sessionIndex[s.id] = s; });
    state.sessions.forEach(function (s) {
      if (s.parent_id && state.sessionIndex[s.parent_id]) {
        state.childCount[s.parent_id] = (state.childCount[s.parent_id] || 0) + 1;
      }
    });
    if (!state.sessions.length) {
      renderListState('empty');
      return;
    }
    if (state.viewMode === 'tree' && treeAllowed()) {
      root.innerHTML = renderTreeSessions();
      return;
    }
    var items = state.sessions.map(function (s) {
      return { group: sessionGroupLabel(s.last_ts), html: buildSessionItem(s) };
    });
    // 非时间排序下日期分组没有意义，直接输出单一列表，避免重复组头。
    if (state.sortMode !== 'recent') {
      root.innerHTML = '<div class="session-group">' +
        items.map(function (it) { return it.html; }).join('') + '</div>';
      return;
    }
    var out = '';
    var i = 0;
    while (i < items.length) {
      var label = items[i].group;
      var j = i;
      while (j < items.length && items[j].group === label) j++;
      out += '<div class="session-group">' +
        '<div class="group-head"><span class="group-label">' + label + '</span>' +
        '<span class="group-count">' + (j - i) + '</span></div>' +
        items.slice(i, j).map(function (it) { return it.html; }).join('') +
        '</div>';
      i = j;
    }
    root.innerHTML = out;
  }

  function updateViewSwitch() {
    var activeView = treeAllowed() ? state.viewMode : 'time';
    $$('#viewSwitch button').forEach(function (btn) {
      var isTree = btn.dataset.view === 'tree';
      btn.classList.toggle('is-on', btn.dataset.view === activeView);
      btn.disabled = isTree && !treeAllowed();
      btn.title = isTree && !treeAllowed() ? '回收站不支持谱系视图' : '';
    });
  }

  function setViewMode(mode) {
    if (mode === 'tree' && !treeAllowed()) {
      toast('回收站不支持谱系视图', 'error');
      return;
    }
    state.viewMode = mode;
    localStorage.setItem('sd.viewMode', mode);
    updateViewSwitch();
    renderSessions();
  }

  function toggleTree(id) {
    var body = $('#listBody');
    var top = body.scrollTop;
    state.treeCollapsed[id] = !state.treeCollapsed[id];
    renderSessions();
    body.scrollTop = top;
  }

  function highlight(text) {
    return esc(text).replace(/\u0001/g, '<mark>').replace(/\u0002/g, '</mark>');
  }

  function renderListState(kind, message) {
    var root = $('#listBody');
    if (kind === 'loading') {
      var rows = '';
      for (var i = 0; i < 4; i++) {
        rows += '<div class="skeleton-item"><span class="sk sk-time"></span>' +
          '<div class="sk-main"><i class="sk sk-title"></i><i class="sk sk-line"></i><i class="sk sk-meta"></i></div></div>';
      }
      root.innerHTML = '<div class="skeleton-list" aria-busy="true" aria-label="正在加载会话">' + rows + '</div>';
      return;
    }
    if (kind === 'error') {
      root.innerHTML = '<div class="state-panel">' +
        '<div class="state-icon state-icon-error">' + ALERT_ICON + '</div>' +
        '<h3>读取会话失败</h3>' +
        '<p>' + esc(message || '未知错误') + '</p>' +
        '<div class="state-actions"><button class="btn btn-primary" data-action="retry">重试</button>' +
        '<button class="btn" data-action="sync">重新扫描</button></div></div>';
      return;
    }
    if (state.folderFilter === 'trash' && !state.query) {
      root.innerHTML = '<div class="state-panel is-empty"><h3>回收站为空</h3></div>';
      return;
    }
    if (state.query) {
      root.innerHTML = '<div class="state-panel">' +
        '<div class="state-icon">' + FOLDER_ICON + '</div>' +
        '<h3>没有匹配的会话</h3>' +
        '<p>换一个关键词，或清除搜索后查看全部。</p>' +
        '<div class="state-actions"><button class="btn btn-primary" data-action="clear-search">清除搜索</button>' +
        '<button class="btn" data-action="reset">查看全部</button></div></div>';
      return;
    }
    var emptyText = '这个归类还没有会话';
    if (!state.projectId) emptyText = '选择左侧项目';
    else if (state.folderFilter === 'all') emptyText = '这个项目里没有会话';
    else if (state.folderFilter === 'unfiled') emptyText = '没有待归类的会话';
    root.innerHTML = '<div class="state-panel is-empty"><h3>' + emptyText + '</h3></div>';
  }

  /* ---------- 详情 ---------- */

  function clearDetail(reason) {
    state.detail = null;
    var head = $('#detailHead');
    head.innerHTML = '';
    head.classList.add('is-hidden');
    $('#composer').hidden = true;
    if (reason === 'search') {
      $('#thread').innerHTML = '<div class="thread-inner"><div class="thread-empty">' +
        '<h3>没有匹配的会话</h3><p>当前搜索没有结果，可以清除搜索或查看全部。</p>' +
        '<div class="state-actions"><button class="btn btn-primary" data-action="clear-search">清除搜索</button>' +
        '<button class="btn" data-action="reset">查看全部</button></div>' +
        '</div></div>';
      return;
    }
    $('#thread').innerHTML = '<div class="thread-inner"><div class="thread-empty">' +
      '<h3>选择会话</h3><p>从目录中选择一条会话开始阅读，或使用右上角的新建会话。</p>' +
      '</div></div>';
  }

  function selectSession(id) {
    state.sessionId = id;
    localStorage.setItem('sd.lastSession', String(id));
    var mobileSidebar = $('.sidebar');
    if (mobileSidebar) mobileSidebar.classList.remove('is-open');
    try { history.replaceState(null, '', '#session=' + id); } catch (e) { /* 忽略 */ }
    $$('#listBody .session-item').forEach(function (el) {
      el.classList.toggle('is-active', el.dataset.session === id);
    });
    var seq = ++detailSeq;
    $('#detailHead').classList.add('is-hidden');
    $('#composer').hidden = true;
    $('#thread').innerHTML = '<div class="thread-inner"><div class="skeleton-list">' +
      '<div class="skeleton-item"><span class="sk sk-time"></span><div class="sk-main"><i class="sk sk-title"></i><i class="sk sk-line"></i></div></div>' +
      '<div class="skeleton-item"><span class="sk sk-time"></span><div class="sk-main"><i class="sk sk-title"></i><i class="sk sk-line"></i></div></div>' +
      '</div></div>';

    return api('/api/sessions/' + encodeURIComponent(id)).then(function (detail) {
      if (seq !== detailSeq) return;
      state.detail = detail;
      renderDetail(detail);
    }).catch(function (e) {
      if (seq !== detailSeq) return;
      $('#thread').innerHTML = '<div class="thread-inner"><div class="thread-empty">' +
        '<h3>无法打开会话</h3><p>' + esc(e.message) + '</p></div></div>';
    });
  }

  // 详情页顶部概要卡片，四态与列表保持一致。
  function aiCardHtml(detail) {
    var s = detail.session;
    var a = detail.ai;
    var head = '<div class="ai-card-head"><span class="ai-card-title">AI 概要</span></div>';

    if (!a) {
      return '<div class="ai-card is-empty">' + head +
        '<p class="ai-card-hint">还没有生成概要。生成后会显示目标、关键动作与结论，并用于自动归类。</p>' +
        '<div class="ai-card-actions"><button class="btn btn-sm btn-primary" data-ai-generate="' + esc(s.id) + '">生成概要</button></div>' +
        '</div>';
    }
    if (a.status === 'running' || a.status === 'pending') {
      return '<div class="ai-card is-loading">' + head +
        '<div class="ai-skeleton"><i class="sk sk-line"></i><i class="sk sk-line short"></i><i class="sk sk-line"></i></div>' +
        '<p class="ai-card-hint">正在生成概要…</p></div>';
    }
    if (a.status === 'failed') {
      return '<div class="ai-card is-error">' + head +
        '<p class="ai-card-hint">生成失败：' + esc(a.error || '模型调用失败') + '</p>' +
        '<div class="ai-card-actions"><button class="btn btn-sm" data-ai-generate="' + esc(s.id) + '" data-force="1">重试</button></div>' +
        '</div>';
    }

    var topics = (a.topics || []).map(function (t) {
      return '<span class="ai-topic">' + esc(t) + '</span>';
    }).join('');
    var titleRow = '';
    if (!s.custom_title && a.suggested_title) {
      titleRow = '<div class="ai-title-suggest">' +
        '<span class="ai-title-suggest-label">建议标题</span>' +
        '<span class="ai-title-suggest-value">' + esc(a.suggested_title) + '</span>' +
        '<button class="btn btn-sm" data-ai-apply-title="' + esc(s.id) + '" data-title="' + esc(a.suggested_title) + '">采用</button>' +
        '</div>';
    }
    var meta = [];
    if (a.lineage_mode === 'delta') meta.push('增量摘要');
    if (a.lineage_mode === 'inherited') meta.push('继承父会话');
    if (a.drift) meta.push('主题偏移 ' + a.drift.toFixed(2));
    if (a.model) meta.push('模型 ' + esc(a.model));
    if (a.confidence) meta.push('置信度 ' + Math.round(a.confidence * 100) + '%');
    if (a.cost_usd) meta.push('成本 $' + a.cost_usd.toFixed(4));
    if (a.updated_at) meta.push('生成于 ' + fmtClock(new Date(a.updated_at * 1000).toISOString()));

    return '<div class="ai-card">' + head +
      '<p class="ai-card-summary">' + esc(a.summary) + '</p>' +
      (topics ? '<div class="ai-topics">' + topics + '</div>' : '') +
      titleRow +
      '<div class="ai-card-foot"><span class="ai-card-meta">' + meta.join(' · ') + '</span>' +
      '<button class="btn btn-sm" data-ai-generate="' + esc(s.id) + '" data-force="1">重新生成</button></div>' +
      '</div>';
  }

  function aiGenerate(sessionID, force) {
    var body = { ids: [sessionID], force: !!force };
    if (state.projectId) body.project_id = state.projectId;
    api('/api/ai/summarize', { method: 'POST', body: JSON.stringify(body) })
      .then(function (res) {
        toast(res.queued ? '已开始生成概要' : '没有需要生成的会话');
      })
      .catch(function (e) { toast(e.message, 'error'); });
  }

  function applySuggestedTitle(sessionID, title) {
    api('/api/sessions/' + encodeURIComponent(sessionID), {
      method: 'PATCH',
      body: JSON.stringify({ title: title })
    }).then(function () {
      toast('已采用建议标题');
      return loadSessions(true);
    }).then(function () {
      if (state.sessionId === sessionID) selectSession(sessionID);
    }).catch(function (e) { toast(e.message, 'error'); });
  }

  function renderDetail(detail) {
    var s = detail.session;
    var deleted = s.state === 'deleted';
    var active = !!s.active;
    var head = $('#detailHead');
    head.classList.remove('is-hidden');

    var statusClass = deleted ? 'is-deleted' : (active ? 'is-live' : 'is-ready');
    var statusText = deleted ? '已删除' : (active ? '进行中' : '可恢复');

    var meta = '<div class="detail-meta">' +
      '<button class="id-copy" id="btnCopyId" title="复制完整会话 ID">' +
      '<span class="mono">' + esc(s.id.slice(0, 8)) + '</span>' +
      '<svg class="id-copy-ico" viewBox="0 0 16 16" aria-hidden="true"><rect x="5.6" y="5.6" width="7.8" height="7.8" rx="1.1"></rect><path d="M3.6 10.4H3a1 1 0 0 1-1-1V3.4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v.6"></path></svg>' +
      '</button>' +
      '<span>' + s.message_count + ' 条消息</span>' +
      '<span class="mono">' + fmtTokens((s.tokens_input || 0) + (s.tokens_output || 0)) + ' tok</span>' +
      '<span>' + fmtSize(s.file_size) + '</span>' +
      '<span class="mono">CC ' + esc(s.cc_version || '—') + '</span>' +
      (deleted && s.deleted_at
        ? '<span>删除于 ' + fmtDay(new Date(s.deleted_at * 1000).toISOString()) + '</span>'
        : '') +
      (deleted ? '' :
        '<span class="grow"></span>' +
        '<button class="icon-btn sm" id="btnRefreshDetail" title="刷新会话内容" aria-label="刷新会话内容">' +
        '<svg class="ico ico-sm" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M13 8a5 5 0 1 1-1.6-3.7"></path><path d="M13 2.8V5h-2.2"></path></svg>' +
        '</button>' +
        '<button class="icon-btn sm" id="btnExport" title="导出会话" aria-label="导出会话">' +
        '<svg class="ico ico-sm" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2.5v7M4.8 6.3 8 9.5l3.2-3.2M3 12.5h10"></path></svg>' +
        '</button>') +
      '</div>';

    var actions;
    if (deleted) {
      actions = '<div class="detail-actions">' +
        '<button class="btn btn-primary" id="btnRestore">还原会话</button>' +
        '<button class="btn btn-danger-ghost" id="btnPurge">彻底删除</button>' +
        '<span class="grow"></span></div>';
    } else {
      var openButton = active
        ? '<span class="terminal-status" hidden><span class="dot"></span>已在终端</span>'
        : '<button class="btn btn-primary" id="btnOpen" hidden>' + PLAY_ICON + '恢复会话</button>';
      var followHtml = s.display_project_source === 'manual'
        ? '<button class="btn-link" id="btnFollowAuto" title="跟随 /cd 自动切换归属" hidden>跟随 /cd</button>'
        : '';
      actions = '<div class="detail-actions">' + openButton +
        '<button class="btn" id="btnRename" hidden>重命名</button>' +
        '<span class="select-label" hidden>归属</span>' +
        '<select class="select" id="projectSelect" aria-label="归属项目" title="手动选择会锁定归属；可用「跟随 /cd」恢复自动" hidden>' + projectOptionsForSession(s) + '</select>' +
        followHtml +
        '<button class="btn btn-danger-ghost" id="btnDelete" hidden>删除</button>' +
        '</div>';
    }

    head.innerHTML =
      '<div class="lineage" id="lineageBar" hidden></div>' +
      '<div class="title-line">' +
      '<h2 title="' + esc(s.title) + '">' + esc(s.title) + '</h2>' +
      '<span class="detail-status ' + statusClass + '"><span class="s-dot"></span>' + statusText + '</span>' +
      '</div>' + meta + actions;

    $('#thread').innerHTML = '<div class="thread-inner">' + aiCardHtml(detail) + renderMessages(detail) + '</div>';
    renderLineage(s, detail);
    var genBtn = $('#thread [data-ai-generate]');
    if (genBtn) {
      genBtn.addEventListener('click', function () {
        aiGenerate(s.id, genBtn.dataset.force === '1');
      });
    }
    var applyTitleBtn = $('#thread [data-ai-apply-title]');
    if (applyTitleBtn) {
      applyTitleBtn.addEventListener('click', function () {
        applySuggestedTitle(s.id, applyTitleBtn.dataset.title);
      });
    }
    $('#btnCopyId').addEventListener('click', function () { copyText(s.id, '会话 ID 已复制'); });

    var comp = $('#composer');
    if (deleted) {
      comp.hidden = true;
      comp.innerHTML = '';
      $('#btnRestore').addEventListener('click', restoreCurrent);
      $('#btnPurge').addEventListener('click', purgeCurrent);
      return;
    }

    comp.hidden = false;
    comp.innerHTML = '<span class="composer-label">在终端继续</span>' +
      '<code>claude --resume ' + esc(s.id) + '</code>' +
      '<button class="icon-btn sm" id="btnCopy" aria-label="复制恢复命令" title="复制命令">' +
      '<svg class="ico ico-sm" viewBox="0 0 16 16"><rect x="5.6" y="5.6" width="7.8" height="7.8" rx="1.1"></rect><path d="M3.6 10.4H3a1 1 0 0 1-1-1V3.4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v.6"></path></svg>' +
      '</button><span class="grow"></span>';

    if (!active) {
      $('#btnOpen').addEventListener('click', openCurrent);
    }
    $('#btnRename').addEventListener('click', renameCurrent);
    $('#projectSelect').addEventListener('change', moveProject);
    if ($('#btnFollowAuto')) {
      $('#btnFollowAuto').addEventListener('click', followAuto);
    }
    $('#btnRefreshDetail').addEventListener('click', refreshCurrent);
    $('#btnExport').addEventListener('click', openExportDialog);
    $('#btnCopy').addEventListener('click', copyResume);
    $('#btnDelete').addEventListener('click', deleteCurrent);
  }

  // 详情页血缘：根 › 父 › 当前，父链由 /ancestors 补全，分叉点来自 forkedFrom。
  function renderLineage(s, detail) {
    var bar = $('#lineageBar');
    if (!bar) return;
    var direct = state.childCount[s.id] || 0;
    var forkAt = '';
    if (s.fork_point_uuid) {
      var idx = -1;
      (detail.messages || []).forEach(function (m, i) {
        if (m.uuid === s.fork_point_uuid) idx = i;
      });
      forkAt = (idx >= 0 && !detail.truncated)
        ? '从第 ' + (idx + 1) + ' 条消息分叉'
        : '从历史消息分叉';
    }
    if (!s.parent_id) {
      if (!direct) { bar.hidden = true; return; }
      bar.hidden = false;
      bar.innerHTML = '<span class="lineage-cur">' + esc(s.title) + '</span>' +
        '<span class="fork-at">根会话 · ' + direct + ' 个直接派生</span>';
      return;
    }
    bar.hidden = false;
    var parentLabel = s.parent_title ? esc(s.parent_title) : '原会话';
    if (s.parent_state === 'deleted') parentLabel += '<span class="lineage-note">在回收站</span>';
    else if (s.parent_state === 'missing') parentLabel += '<span class="lineage-note">已清理</span>';
    bar.innerHTML = '<span class="lineage-link is-static">' + parentLabel + '</span>' +
      '<span class="lineage-sep">' + CHEV_RIGHT_ICON + '</span>' +
      '<span class="lineage-cur">' + esc(s.title) + '</span>' +
      (forkAt ? '<span class="fork-at">' + forkAt + '</span>' : '');

    var seq = detailSeq;
    api('/api/sessions/' + encodeURIComponent(s.id) + '/ancestors').then(function (data) {
      if (seq !== detailSeq) return;
      var chain = data.ancestors || [];
      if (!chain.length) return;
      var html = chain.map(function (a) {
        var label = esc(a.title);
        if (a.state === 'deleted') label += '<span class="lineage-note">在回收站</span>';
        else if (a.state === 'missing') label += '<span class="lineage-note">已清理</span>';
        var inList = !!state.sessionIndex[a.id];
        return '<button class="lineage-link' + (inList ? '' : ' is-static') + '"' +
          (inList ? ' data-lineage="' + esc(a.id) + '"' : '') +
          ' title="' + esc(a.title) + '">' + label + '</button>';
      }).join('<span class="lineage-sep">' + CHEV_RIGHT_ICON + '</span>');
      bar.innerHTML = html + '<span class="lineage-sep">' + CHEV_RIGHT_ICON + '</span>' +
        '<span class="lineage-cur">' + esc(s.title) + '</span>' +
        (forkAt ? '<span class="fork-at">' + forkAt + '</span>' : '');
    }).catch(function () { /* 链路加载失败时保留父标题占位 */ });
  }

  function projectOptionsForSession(s) {
    var current = s.display_project_id || s.project_id;
    return state.projects.map(function (p) {
      return '<option value="' + p.id + '"' + (p.id === current ? ' selected' : '') + '>' + esc(p.name) + '</option>';
    }).join('');
  }

  function moveProject(e) {
    var s = state.detail && state.detail.session;
    if (!s) return;
    var target = Number(e.target.value);
    api('/api/sessions/' + encodeURIComponent(s.id), {
      method: 'PATCH',
      body: JSON.stringify({ project_id: target })
    }).then(function () {
      toast('已归属到项目');
      return loadProjects();
    }).then(function () {
      if (target !== state.projectId) {
        return selectProject(target, true);
      }
      return loadSessions(true);
    }).then(function () {
      return selectSession(s.id);
    }).catch(function (err) {
      toast(err.message, 'error');
    });
  }

  function followAuto() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    api('/api/sessions/' + encodeURIComponent(s.id), {
      method: 'PATCH',
      body: JSON.stringify({ follow_auto: true })
    }).then(function (updated) {
      toast('已恢复自动归属');
      return loadProjects().then(function () {
        var target = updated.display_project_id || updated.project_id;
        if (target !== state.projectId) {
          return selectProject(target, true);
        }
        return loadSessions(true);
      }).then(function () {
        return selectSession(s.id);
      });
    }).catch(function (err) {
      toast(err.message, 'error');
    });
  }

  function renderMarkdown(text) {
    var src = String(text || '').replace(/\r\n/g, '\n');
    var codeBlocks = [];
    src = src.replace(/```([^\n]*)\n([\s\S]*?)```/g, function (_, lang, code) {
      codeBlocks.push({ lang: String(lang || '').trim(), code: code });
      return '\u0000CODE' + (codeBlocks.length - 1) + '\u0000';
    });

    var lines = src.split('\n');
    var out = [];
    var inList = false;
    var i = 0;

    function closeList() {
      if (inList) {
        out.push('</ul>');
        inList = false;
      }
    }
    function isListItem(line) {
      return /^\s*(?:[-*]|\d+\.)\s+/.test(line);
    }
    function isHeading(line) {
      return /^#{1,4}\s+/.test(line);
    }
    function isTableStart(idx) {
      return /^\s*\|.+\|\s*$/.test(lines[idx] || '') &&
        /^\s*\|[\s:|-]+\|\s*$/.test(lines[idx + 1] || '');
    }

    while (i < lines.length) {
      var line = lines[i];
      var codeMatch = line.match(/^\u0000CODE(\d+)\u0000$/);
      if (codeMatch) {
        closeList();
        var cb = codeBlocks[Number(codeMatch[1])];
        out.push('<pre class="md-code"><code>' + esc(cb.code) + '</code></pre>');
        i++;
        continue;
      }
      if (isTableStart(i)) {
        closeList();
        var header = splitTableRow(lines[i]);
        var rows = [];
        var j = i + 2;
        while (j < lines.length && /^\s*\|.+\|\s*$/.test(lines[j])) {
          rows.push(splitTableRow(lines[j]));
          j++;
        }
        var table = '<table class="md-table"><thead><tr>' +
          header.map(function (h) { return '<th>' + mdInline(h) + '</th>'; }).join('') +
          '</tr></thead><tbody>';
        rows.forEach(function (r) {
          table += '<tr>' + r.map(function (c) { return '<td>' + mdInline(c) + '</td>'; }).join('') + '</tr>';
        });
        out.push(table + '</tbody></table>');
        i = j;
        continue;
      }
      var heading = line.match(/^(#{1,4})\s+(.*)$/);
      if (heading) {
        closeList();
        var level = Math.min(6, heading[1].length + 2);
        out.push('<h' + level + '>' + mdInline(heading[2]) + '</h' + level + '>');
        i++;
        continue;
      }
      if (/^\s*---+\s*$/.test(line)) {
        closeList();
        out.push('<hr>');
        i++;
        continue;
      }
      if (isListItem(line)) {
        if (!inList) {
          out.push('<ul>');
          inList = true;
        }
        out.push('<li>' + mdInline(line.replace(/^\s*(?:[-*]|\d+\.)\s+/, '')) + '</li>');
        i++;
        continue;
      }
      if (!line.trim()) {
        closeList();
        i++;
        continue;
      }
      var para = [line];
      i++;
      while (i < lines.length && lines[i].trim() && !isHeading(lines[i]) &&
        !isListItem(lines[i]) && !isTableStart(i) &&
        !/^\u0000CODE\d+\u0000$/.test(lines[i]) &&
        !/^\s*---+\s*$/.test(lines[i])) {
        para.push(lines[i]);
        i++;
      }
      closeList();
      out.push('<p>' + mdInline(para.join('\n')) + '</p>');
    }
    closeList();
    return out.join('');
  }

  function splitTableRow(line) {
    return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map(function (s) {
      return s.trim();
    });
  }

  function mdInline(text) {
    var s = esc(text);
    s = s.replace(/`([^`]+)`/g, '<code>$1</code>');
    s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    s = s.replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em>$2</em>');
    s = s.replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
    s = s.replace(/\n/g, '<br>');
    return s;
  }

  function renderMessages(detail) {
    var msgs = detail.messages || [];
    if (!msgs.length) {
      return '<div class="thread-empty"><h3>没有可展示的消息</h3><p>这个会话文件里只有元数据或系统消息。</p></div>';
    }
    var html = '';
    var lastDay = '';
    msgs.forEach(function (m) {
      var day = dayKey(m.timestamp);
      if (day !== lastDay) {
        html += '<div class="day-rule"><span>' + fmtDay(m.timestamp) + '</span></div>';
        lastDay = day;
      }
      var user = m.type === 'user';
      html += '<article class="msg">' +
        '<div class="avatar' + (user ? '' : ' avatar-ai') + '">' + (user ? '你' : 'AI') + '</div>' +
        '<div class="msg-body">' +
        '<div class="msg-head"><span class="who">' + (user ? '你' : 'Claude') + '</span>' +
        (user ? '' : '<span class="model-tag">assistant</span>') +
        '<time>' + fmtClock(m.timestamp) + '</time></div>' +
        (m.text ? '<div class="msg-text">' + renderMarkdown(m.text) + '</div>' : '') +
        (m.tools || []).map(renderTool).join('') +
        '</div></article>';
    });
    html += '<div class="sys-note"><span class="dot"></span><span>' +
      (detail.truncated ? '仅显示最近 300 条消息' : '共 ' + msgs.length + ' 条消息') +
      '</span><span class="grow"></span><span class="mono">' + esc(detail.session.cc_version || '') + '</span></div>';
    return html;
  }

  function summarizeTool(input) {
    try {
      var obj = JSON.parse(input);
      if (obj && typeof obj === 'object') {
        var keys = ['command', 'file_path', 'path', 'pattern', 'url', 'query', 'description'];
        for (var i = 0; i < keys.length; i++) {
          if (typeof obj[keys[i]] === 'string' && obj[keys[i]]) return obj[keys[i]];
        }
      }
    } catch (e) { /* 截断的 JSON，走正则兜底 */ }
    var m = input.match(/"(?:file_path|path|command|pattern|url|query)"\s*:\s*"([\s\S]*?)(?:"\s*[,}]|$)/);
    if (m) {
      return m[1].replace(/\\"/g, '"').replace(/\\\\/g, '\\').replace(/\\n/g, ' ').replace(/\s+/g, ' ').trim();
    }
    return input.split('\n')[0];
  }

  function prettyToolInput(input) {
    try { return JSON.stringify(JSON.parse(input), null, 2); } catch (e) { return input; }
  }

  function renderTool(t) {
    var input = t.input || '';
    var summary = summarizeTool(input).slice(0, 160);
    return '<details class="tool-call"><summary>' +
      '<span class="tool-kind">' + esc(t.name || 'tool') + '</span>' +
      '<span class="tool-cmd">' + esc(summary) + '</span></summary>' +
      '<pre>' + esc(prettyToolInput(input)) + '</pre></details>';
  }

  function openCurrent() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    var btn = $('#btnOpen');
    btn.classList.add('is-loading');
    btn.disabled = true;
    api('/api/sessions/' + encodeURIComponent(s.id) + '/open', { method: 'POST' }).then(function (res) {
      var term = res.result && res.result.terminal === 'windows-terminal' ? 'Windows Terminal' : '新终端窗口';
      toast('已在 ' + term + ' 打开');
    }).catch(function (e) {
      toast(e.message, 'error');
    }).then(function () {
      btn.classList.remove('is-loading');
      btn.disabled = false;
    });
  }

  function renameCurrent() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    promptDialog({ title: '重命名会话', label: '标题', value: s.title }).then(function (title) {
      if (title === null) return;
      if (!title) {
        toast('标题不能为空', 'error');
        return;
      }
      api('/api/sessions/' + encodeURIComponent(s.id), {
        method: 'PATCH',
        body: JSON.stringify({ title: title })
      }).then(function () {
        toast('标题已更新');
        return loadSessions().then(function () { return selectSession(s.id); });
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function refreshCurrent() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    var btn = $('#btnRefreshDetail');
    if (btn) {
      if (btn.classList.contains('is-loading')) return;
      btn.classList.add('is-loading');
    }
    var seq = ++detailSeq;
    var scroll = $('#thread').scrollTop;
    api('/api/sessions/' + encodeURIComponent(s.id)).then(function (detail) {
      if (seq !== detailSeq) return;
      state.detail = detail;
      renderDetail(detail);
      $('#thread').scrollTop = scroll;
      toast('会话内容已刷新');
    }).catch(function (e) {
      toast(e.message, 'error');
    }).then(function () {
      var current = $('#btnRefreshDetail');
      if (current) current.classList.remove('is-loading');
    });
  }

  function openExportDialog() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    var root = $('#modalRoot');
    root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
      '<header class="modal-head"><div><h3>导出会话</h3>' +
      '<p>JSON 导出原始 jsonl 内容，Markdown 导出渲染后的对话。</p></div>' +
      '<button class="icon-btn" data-close aria-label="关闭">' +
      '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
      '<div class="modal-body export-options">' +
      '<button class="btn" data-export="json">导出原始 JSON（.jsonl）</button>' +
      '<button class="btn" data-export="md">导出 Markdown（.md）</button>' +
      '</div></div></div>';
    function close() { root.innerHTML = ''; }
    root.querySelector('[data-close]').addEventListener('click', close);
    root.querySelector('.modal-backdrop').addEventListener('click', function (e) {
      if (e.target === e.currentTarget) close();
    });
    root.querySelectorAll('[data-export]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var format = btn.dataset.export;
        close();
        var a = document.createElement('a');
        a.href = '/api/sessions/' + encodeURIComponent(s.id) + '/export?format=' + format;
        a.download = '';
        document.body.appendChild(a);
        a.click();
        a.remove();
      });
    });
  }

  function copyText(text, message) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function () {
        toast(message);
      }).catch(function () {
        toast('复制失败，请手动选择文本', 'error');
      });
    } else {
      toast('当前浏览器不支持剪贴板', 'error');
    }
  }

  function copyResume() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    copyText('claude --resume ' + s.id, '恢复命令已复制');
    var btn = $('#btnCopy');
    btn.classList.add('is-done');
    setTimeout(function () { btn.classList.remove('is-done'); }, 1200);
  }

  /* ---------- 同步与统计 ---------- */

  function doSync() {
    var btn = $('#btnSync');
    if (btn.classList.contains('is-loading')) return;
    btn.classList.add('is-loading');
    btn.disabled = true;
    setSync('正在扫描', false);
    api('/api/sync', { method: 'POST' }).then(function (res) {
      setSync('已同步', false);
      var msg = '扫描完成：' + res.sessions + ' 个会话';
      if (res.errors && res.errors.length) msg += '，' + res.errors.length + ' 个文件有问题';
      toast(msg);
      return loadProjects().then(function () {
        if (state.projectId) return Promise.all([loadFolders(), loadSessions()]);
      }).then(refreshStats);
    }).catch(function (e) {
      setSync('同步失败', true);
      toast(e.message, 'error');
    }).then(function () {
      btn.classList.remove('is-loading');
      btn.disabled = false;
    });
  }

  /* ---------- AI 概要 ---------- */

  function pollAIJob(jobID, opts) {
    opts = opts || {};
    var start = Date.now();
    var lastChange = Date.now();
    var lastProcessed = -1;
    return new Promise(function (resolve, reject) {
      function tick() {
        api('/api/ai/jobs/' + encodeURIComponent(jobID)).then(function (job) {
          if (opts.onProgress) opts.onProgress(job);
          var processed = (job.done || 0) + (job.failed || 0);
          if (processed !== lastProcessed) {
            lastProcessed = processed;
            lastChange = Date.now();
          }
          if (job.status === 'done' || job.status === 'failed' || job.status === 'canceled') {
            resolve(job);
            return;
          }
          if (Date.now() - start > (opts.timeoutMs || 10 * 60 * 1000)) {
            reject(new Error('任务超时，请稍后重试'));
            return;
          }
          if (Date.now() - lastChange > (opts.stallMs || 15 * 60 * 1000)) {
            reject(new Error('任务长时间没有进展，请稍后重试'));
            return;
          }
          setTimeout(tick, 1500);
        }).catch(reject);
      }
      tick();
    });
  }

  // AI 整理按钮按项目记录忙碌状态，切换项目时显示对应项目的进度。
  function syncAITidyButton() {
    var btn = $('#btnAITidy');
    if (!btn) return;
    var busy = state.projectId ? state.aiBusy[state.projectId] : null;
    if (!busy) {
      btn.disabled = false;
      btn.textContent = 'AI 整理';
      return;
    }
    btn.disabled = true;
    var label = busy.phase === 'summary' ? '生成概要' : '整理归类';
    var total = busy.total || 0;
    var processed = Math.min(total, (busy.done || 0) + (busy.failed || 0));
    btn.textContent = total ? label + ' ' + processed + '/' + total : label + '…';
  }

  function setAIBusy(projectID, phase, total) {
    if (!projectID) return;
    state.aiBusy[projectID] = { phase: phase, done: 0, failed: 0, total: total || 0 };
    syncAITidyButton();
  }

  function updateAIProgress(projectID, job) {
    var busy = state.aiBusy[projectID];
    if (!busy || !job) return;
    busy.done = job.done || 0;
    busy.failed = job.failed || 0;
    if (job.total) busy.total = job.total;
    if (state.projectId === projectID) syncAITidyButton();
  }

  function clearAIBusy(projectID) {
    if (!projectID) return;
    delete state.aiBusy[projectID];
    syncAITidyButton();
  }

  function projectDisplayName(projectID) {
    var project = state.projects.filter(function (p) { return p.id === projectID; })[0];
    return project ? project.name : '#' + projectID;
  }

  // AI 整理：先生成缺失概要，再做项目级归类，最后打开预览。
  function aiTidy() {
    if (!state.projectId) { toast('请先选择一个项目', 'error'); return; }
    var projectID = state.projectId;
    if (state.aiBusy[projectID]) return;
    var projectName = projectDisplayName(projectID);

    var hasAIFolders = (state.folders || []).some(function (f) { return f.source === 'ai'; });
    var ask = hasAIFolders
      ? confirmDialog({
        title: '重新整理归类',
        message: '项目中已有 AI 创建的归类。选「重新评估」会把这些会话重新分配；选「取消」则只整理未归类的会话。手动整理的会话始终不受影响。',
        confirmText: '重新评估'
      })
      : Promise.resolve(false);

    ask.then(function (reevaluate) {
      setAIBusy(projectID, 'summary');
      return api('/api/ai/summarize', {
        method: 'POST',
        body: JSON.stringify({ project_id: projectID })
      }).then(function (res) {
        if (!res.job_id) throw new Error('入队失败');
        setAIBusy(projectID, 'summary', res.queued || 0);
        return pollAIJob(res.job_id, {
          timeoutMs: 60 * 60 * 1000,
          stallMs: 12 * 60 * 1000,
          onProgress: function (job) { updateAIProgress(projectID, job); }
        }).then(function (job) {
          if (job.status === 'failed') throw new Error(job.error || '概要生成失败');
          if (job.failed > 0) {
            toast('有 ' + job.failed + ' 个会话概要生成失败，将继续整理已有概要的会话', 'error');
          }
          return reevaluate;
        });
      });
    }).then(function (reevaluate) {
      setAIBusy(projectID, 'categorize');
      return api('/api/ai/categorize', {
        method: 'POST',
        body: JSON.stringify({ project_id: projectID, reevaluate: !!reevaluate })
      }).then(function (res) {
        if (!res.job_id) throw new Error('入队失败');
        setAIBusy(projectID, 'categorize', res.total || 0);
        return pollAIJob(res.job_id, {
          timeoutMs: 30 * 60 * 1000,
          stallMs: 6 * 60 * 1000,
          onProgress: function (job) { updateAIProgress(projectID, job); }
        }).then(function (job) {
          refreshAIStatus();
          if (job.status === 'failed') throw new Error(job.error || '归类失败');
          if (job.failed > 0) {
            toast('有 ' + job.failed + ' 个会话未能完成归类，已保留其余结果', 'error');
          }
          if (!job.total) {
            return infoDialog({
              title: '没有可整理的会话',
              message: res.reason || '当前项目没有可整理的会话。'
            });
          }
          return openSuggestionDialog(res.job_id, projectName);
        });
      });
    }).catch(function (e) {
      toast(e.message, 'error');
    }).then(function () {
      clearAIBusy(projectID);
    });
  }

  function openSuggestionDialog(jobID, projectName) {
    return api('/api/ai/suggestions?job_id=' + encodeURIComponent(jobID)).then(function (data) {
      if (data.job && data.job.status === 'failed') {
        return infoDialog({
          title: 'AI 整理失败',
          message: data.job.error || '模型调用失败，请稍后重试。'
        });
      }
      var folders = data.folders || [];
      var assignments = data.assignments || [];
      var root = $('#modalRoot');

      var groups = {};
      folders.forEach(function (f) {
        var path = (f.parent_path || []).concat([f.name]);
        var key = path.join('/');
        groups[key] = { path: path, folder: f, items: [] };
      });
      var kept = assignments.filter(function (a) {
        return !(a.folder_path || []).length;
      });
      assignments.forEach(function (a) {
        var path = a.folder_path || [];
        if (!path.length) return;
        var key = path.join('/');
        if (!groups[key]) groups[key] = { path: path, folder: null, items: [] };
        groups[key].items.push(a);
      });

      var list = Object.keys(groups).map(function (k) { return groups[k]; });
      list.sort(function (a, b) {
        if (!!b.folder !== !!a.folder) return a.folder ? -1 : 1;
        return a.path.join('/').localeCompare(b.path.join('/'));
      });

      function sessionTitle(sid) {
        var item = state.sessionIndex[sid];
        return item ? item.title : sid.slice(0, 8);
      }

      var bodyHtml = list.map(function (g) {
        var lowCount = g.items.filter(function (a) { return a.confidence < 0.6; }).length;
        var badge = '';
        if (g.folder) {
          var parentPath = g.folder.parent_path || [];
          badge = parentPath.length
            ? '<span class="ai-sug-new">挂到 ' + esc(parentPath.join(' / ')) + ' 下</span>'
            : '<span class="ai-sug-new">新建</span>';
        } else {
          badge = '<span class="ai-sug-reuse">复用已有</span>';
        }
        var header = '<div class="ai-sug-head">' +
          '<span class="ai-sug-check" data-group-check="' + esc(g.path.join('/')) + '"></span>' +
          '<span class="ai-sug-path">' + g.path.map(esc).join('<span class="ai-sug-slash">/</span>') + '</span>' +
          badge +
          '<span class="ai-sug-count">' + g.items.length + ' 个会话' +
          (lowCount ? ' · ' + lowCount + ' 个低置信度' : '') + '</span></div>';

        var desc = g.folder && g.folder.description
          ? '<p class="ai-sug-desc">' + esc(g.folder.description) + '</p>' : '';

        var items = g.items.map(function (a) {
          var low = a.confidence < 0.6;
          return '<label class="ai-sug-item' + (low ? ' is-low' : '') + '">' +
            '<input type="checkbox" class="ai-sug-box" data-session-sug="' + a.id + '"' +
            (low ? '' : ' checked') + '>' +
            '<span class="ai-sug-body"><span class="ai-sug-title">' + esc(sessionTitle(a.session_id)) + '</span>' +
            '<span class="ai-sug-reason">' + esc(a.reason || '') +
            (low ? '（置信度较低，默认不勾选，可按需选中）' : '') + '</span></span>' +
            '<span class="ai-sug-conf">' + (a.confidence || 0).toFixed(2) + '</span></label>';
        }).join('');

        return '<div class="ai-sug-group">' + header + desc + '<div class="ai-sug-items">' + items + '</div></div>';
      }).join('');

      var keptHtml = '';
      if (kept.length) {
        keptHtml = '<div class="ai-sug-group">' +
          '<div class="ai-sug-head"><span class="ai-sug-path">保持未归类</span>' +
          '<span class="ai-sug-count">' + kept.length + ' 个会话</span></div>' +
          '<div class="ai-sug-items">' +
          kept.map(function (a) {
            return '<div class="ai-sug-item is-low"><span class="ai-sug-body">' +
              '<span class="ai-sug-title">' + esc(sessionTitle(a.session_id)) + '</span>' +
              '<span class="ai-sug-reason">' + esc(a.reason || '') + '</span></span></div>';
          }).join('') +
          '</div></div>';
      }

      var contentHtml = bodyHtml + keptHtml;
      if (!contentHtml) {
        contentHtml = '<p class="dialog-text">没有可应用的归类建议。</p>';
      }

      var newCount = folders.length;
      var moveCount = assignments.filter(function (a) {
        return (a.folder_path || []).length > 0;
      }).length;

      root.innerHTML = '<div class="modal-backdrop"><div class="modal ai-sug-modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>AI 整理预览' +
        (projectName ? ' · ' + esc(projectName) : '') + '</h3>' +
        '<p class="modal-sub">新建 ' + newCount + ' 个归类，移动 ' + moveCount + ' 个会话。' +
        (kept.length ? '另有 ' + kept.length + ' 个会话保持未归类。' : '') +
        '手动整理的会话已排除。</p></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body ai-sug-body">' +
        '<div class="ai-sug-tools"><button class="btn btn-sm" data-sug-all>全选</button>' +
        '<button class="btn btn-sm" data-sug-none>清空</button></div>' +
        contentHtml + '</div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-discard>丢弃本次</button>' +
        '<button class="btn" data-close>关闭</button>' +
        '<button class="btn btn-primary" data-apply>应用选中</button></footer>' +
        '</div></div>';

      var backdrop = root.querySelector('.modal-backdrop');
      function close() { root.innerHTML = ''; }
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) close(); });
      root.querySelectorAll('[data-close]').forEach(function (el) {
        el.addEventListener('click', close);
      });

      function boxes() { return root.querySelectorAll('.ai-sug-box:not([disabled])'); }
      root.querySelector('[data-sug-all]').addEventListener('click', function () {
        boxes().forEach(function (b) { b.checked = true; });
        syncGroupChecks();
      });
      root.querySelector('[data-sug-none]').addEventListener('click', function () {
        boxes().forEach(function (b) { b.checked = false; });
        syncGroupChecks();
      });
      function syncGroupChecks() {
        root.querySelectorAll('.ai-sug-group').forEach(function (g) {
          var all = g.querySelectorAll('.ai-sug-box:not([disabled])');
          var checked = g.querySelectorAll('.ai-sug-box:not([disabled]):checked');
          var mark = g.querySelector('.ai-sug-check');
          if (mark) mark.classList.toggle('is-on', all.length > 0 && all.length === checked.length);
        });
      }
      root.querySelectorAll('.ai-sug-check').forEach(function (el) {
        el.addEventListener('click', function () {
          var key = el.dataset.groupCheck;
          var group = list.filter(function (g) { return g.path.join('/') === key; })[0];
          if (!group) return;
          var on = !el.classList.contains('is-on');
          root.querySelectorAll('.ai-sug-box:not([disabled])').forEach(function (b) {
            var item = group.items.filter(function (a) { return String(a.id) === b.dataset.sessionSug; })[0];
            if (item) b.checked = on;
          });
          syncGroupChecks();
        });
      });
      root.querySelectorAll('.ai-sug-box').forEach(function (b) {
        b.addEventListener('change', syncGroupChecks);
      });
      syncGroupChecks();

      root.querySelector('[data-discard]').addEventListener('click', function () {
        api('/api/ai/suggestions/discard', {
          method: 'POST', body: JSON.stringify({ job_id: jobID })
        }).then(function () {
          toast('已丢弃本次建议');
          close();
        }).catch(function (e) { toast(e.message, 'error'); });
      });

      root.querySelector('[data-apply]').addEventListener('click', function () {
        var sessionIDs = [];
        root.querySelectorAll('.ai-sug-box:checked').forEach(function (b) {
          sessionIDs.push(parseInt(b.dataset.sessionSug, 10));
        });
        api('/api/ai/suggestions/apply', {
          method: 'POST',
          body: JSON.stringify({ job_id: jobID, session_ids: sessionIDs })
        }).then(function (res) {
          toast('已应用 ' + res.applied + ' 个会话');
          close();
          return Promise.all([loadFolders(), loadSessions()]);
        }).catch(function (e) { toast(e.message, 'error'); });
      });
    });
  }

  function refreshAIStatus() {
    var line = $('#aiStatusLine');
    if (!line) return Promise.resolve();
    var query = state.projectId ? '?project_id=' + state.projectId : '';
    return api('/api/ai/status' + query).then(function (st) {
      state.aiStatus = st;
      if (st.probed === false) {
        line.textContent = '待探测';
        line.title = '首次使用 AI 整理时自动探测，也可在 AI 设置中手动探测';
        return;
      }
      if (!st.available) {
        line.textContent = '不可用';
        line.title = st.error || '';
        return;
      }
      var pending = st.pending || 0;
      line.textContent = pending > 0 ? ('待生成 ' + pending) : (st.summary_count + ' 条');
      line.title = '档位 ' + (st.profile || '—') + '，累计 $' + (st.cost_usd || 0).toFixed(3);
    }).catch(function () {});
  }

  function aiSettingsDialog() {
    Promise.all([
      api('/api/ai/settings'),
      api('/api/ai/status' + (state.projectId ? '?project_id=' + state.projectId : ''))
    ]).then(function (res) {
      var cfg = res[0] || {};
      var st = res[1] || {};
      var root = $('#modalRoot');
      function sw(id, on) {
        return '<button class="ai-switch' + (on ? ' is-on' : '') + '" id="' + id + '" role="switch" ' +
          'aria-checked="' + (on ? 'true' : 'false') + '"><span></span></button>';
      }
      function row(label, control, hint) {
        return '<div class="ai-form-row"><span class="ai-form-label">' + esc(label) + '</span>' +
          '<div class="ai-form-control">' + control +
          (hint ? '<span class="ai-form-hint">' + esc(hint) + '</span>' : '') + '</div></div>';
      }
      var profile = st.probed === false ? '待探测' : (st.available ? (st.profile || '—') : '不可用');

      root.innerHTML = '<div class="modal-backdrop"><div class="modal ai-settings" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>AI 设置</h3>' +
        '<p class="modal-sub">通过本机 Claude Code 生成会话概要</p></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body ai-form">' +
        row('启用 AI', sw('aiEnabled', cfg.enabled !== false), '生成会话概要，供列表与归类使用') +
        row('模型', '<input class="ai-input wide" id="aiModel" value="' + esc(cfg.model || '') +
          '" placeholder="留空使用默认模型">', 'CLI 可能被网关覆盖，实际模型以运行结果为准') +
        row('能力档位', '<span class="ai-profile" id="aiProfile">' + esc(profile) + '</span>' +
          '<button class="btn btn-sm" id="aiReprobe">重新探测</button>',
          st.version ? 'CLI ' + st.version : '') +
        row('最小消息数', '<input class="ai-input short" id="aiMinMessages" type="number" min="1" value="' +
          (cfg.min_messages || 2) + '">', '少于该条数的会话不生成概要') +
        row('字符预算', '<input class="ai-input short" id="aiMaxChars" type="number" min="300" value="' +
          (cfg.max_chars || 1500) + '">', '采样上限，约 900 token') +
        row('单轮入队上限', '<input class="ai-input short" id="aiLimit" type="number" min="1" value="' +
          (cfg.auto_enqueue_limit || 50) + '">', '仅当前项目内生效') +
        row('批量大小', '<input class="ai-input short" id="aiBatch" type="number" min="1" max="20" value="' +
          (cfg.summary_batch_size || 15) + '">', '一次请求处理多少个会话；越大越快，但失败时拆成单条重试') +
        row('附带结论段', sw('aiTail', cfg.include_conclusion_tail !== false),
          '追加最后一条回答的结论，避免只得到问题清单') +
        row('排除项目', '<input class="ai-input wide" id="aiExclude" value="' +
          esc((cfg.exclude_projects || []).join('，')) + '" placeholder="项目路径，逗号分隔">',
          '这些项目的会话不会被采样') +
        '<div class="ai-usage">' +
        '<span>已生成 <b>' + (st.summary_count || 0) + '</b></span>' +
        '<span>失败 <b>' + (st.failed || 0) + '</b></span>' +
        '<span>待生成 <b>' + (st.pending || 0) + '</b></span>' +
        '<span>累计成本 <b>$' + (st.cost_usd || 0).toFixed(3) + '</b></span>' +
        '</div></div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn btn-primary" data-save>保存</button></footer>' +
        '</div></div>';

      var backdrop = root.querySelector('.modal-backdrop');
      function close() { root.innerHTML = ''; }
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) close(); });
      root.querySelector('[data-close]').addEventListener('click', close);
      root.querySelector('[data-cancel]').addEventListener('click', close);
      root.querySelectorAll('.ai-switch').forEach(function (el) {
        el.addEventListener('click', function () {
          el.classList.toggle('is-on');
          el.setAttribute('aria-checked', el.classList.contains('is-on') ? 'true' : 'false');
        });
      });

      root.querySelector('#aiReprobe').addEventListener('click', function () {
        var btn = this;
        btn.disabled = true;
        btn.textContent = '探测中…';
        api('/api/ai/probe', { method: 'POST' }).then(function (out) {
          if (out.available) {
            root.querySelector('#aiProfile').textContent = out.profile || '—';
            toast('AI 可用：' + (out.profile || ''));
          } else {
            root.querySelector('#aiProfile').textContent = '不可用';
            toast(out.error || 'AI 不可用', 'error');
          }
        }).catch(function (e) { toast(e.message, 'error'); }).then(function () {
          btn.disabled = false;
          btn.textContent = '重新探测';
        });
      });

      root.querySelector('[data-save]').addEventListener('click', function () {
        var payload = {
          enabled: root.querySelector('#aiEnabled').classList.contains('is-on'),
          model: root.querySelector('#aiModel').value.trim(),
          min_messages: parseInt(root.querySelector('#aiMinMessages').value, 10) || 2,
          max_chars: parseInt(root.querySelector('#aiMaxChars').value, 10) || 1500,
          auto_enqueue_limit: parseInt(root.querySelector('#aiLimit').value, 10) || 50,
          summary_batch_size: parseInt(root.querySelector('#aiBatch').value, 10) || 15,
          include_conclusion_tail: root.querySelector('#aiTail').classList.contains('is-on'),
          exclude_projects: root.querySelector('#aiExclude').value.split(/[，,]/)
            .map(function (s) { return s.trim(); })
            .filter(function (s) { return !!s; })
        };
        api('/api/ai/settings', { method: 'PATCH', body: JSON.stringify(payload) })
          .then(function () {
            toast('AI 设置已保存');
            close();
            refreshAIStatus();
          })
          .catch(function (e) { toast(e.message, 'error'); });
      });
    }).catch(function (e) { toast(e.message, 'error'); });
  }

  function refreshStats() {
    return api('/api/health').then(function (h) {
      $('#statLine').textContent = '项目 ' + h.projects + '，会话 ' + h.sessions;
      if (h.last_scan) {
        $('#scanLine').textContent = '最后扫描 ' + fmtClock(h.last_scan);
        setSync('已同步 ' + fmtClock(h.last_scan), false);
      } else {
        $('#scanLine').textContent = '尚未扫描';
        setSync('未扫描', false);
      }
      refreshAIStatus();
    }).catch(function () {});
  }

  /* ---------- 添加项目 ---------- */

  function addProject() {
    projectDialog().then(function (path) {
      if (path === null) return;
      if (!path) {
        toast('路径不能为空', 'error');
        return;
      }
      api('/api/projects', { method: 'POST', body: JSON.stringify({ path: path }) }).then(function (project) {
        toast('项目已添加');
        return loadProjects().then(function () {
          if (state.projectId === project.id) {
            return loadFolders().then(loadSessions);
          }
          return selectProject(project.id, true);
        });
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function toggleProjectPin(id, pinned) {
    api('/api/projects/' + id, {
      method: 'PATCH',
      body: JSON.stringify({ pinned: !pinned })
    }).then(function () {
      toast(!pinned ? '已收藏项目' : '已取消收藏');
      return loadProjects();
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  function movePinnedProject(id, direction) {
    api('/api/projects/' + id + '/move', {
      method: 'POST',
      body: JSON.stringify({ direction: direction })
    }).then(function () {
      return loadProjects();
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  function editProjectAlias(id) {
    var project = state.projects.filter(function (p) { return p.id === id; })[0];
    if (!project) return;
    promptDialog({
      title: '设置项目别名',
      label: '别名',
      value: project.alias || project.name,
      placeholder: '留空则恢复原名「' + (project.original_name || project.name) + '」'
    }).then(function (alias) {
      if (alias === null) return;
      api('/api/projects/' + id, {
        method: 'PATCH',
        body: JSON.stringify({ alias: alias })
      }).then(function () {
        toast(alias ? '别名已更新' : '已恢复原项目名');
        return loadProjects();
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function deleteProject(id) {
    var project = state.projects.filter(function (p) { return p.id === id; })[0];
    if (!project) return;
    confirmDialog({
      title: '删除项目',
      message: '将从CC会话管理移除「' + project.name + '」及其 ' + project.session_count +
        ' 个会话。会话文件会移动到本机回收站，磁盘上的项目文件夹不会被修改。',
      confirmText: '删除项目',
      danger: true
    }).then(function (ok) {
      if (!ok) return;
      api('/api/projects/' + id, { method: 'DELETE' }).then(function (res) {
        toast('项目已删除' + (res.sessions ? '，已移走 ' + res.sessions + ' 个会话' : ''));
        try { localStorage.removeItem('sd.folderFilter.' + id); } catch (e) { /* 隐私模式 */ }
        delete state.aiBusy[id];
        if (state.projectId === id) {
          state.projectId = null;
          state.sessionId = null;
          clearDetail();
        }
        return loadProjects();
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function projectDialog() {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>添加项目</h3>' +
        '<p>登记一个 Claude Code 工作目录。即使还没有会话，也可以先加进来。</p></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body">' +
        '<div class="field"><label for="pdPath">项目目录</label>' +
        '<div class="path-row">' +
        '<input class="text-input" id="pdPath" autocomplete="off" placeholder="D:\\workspace\\go\\my-project">' +
        '<button class="btn" id="pdBrowse">浏览…</button>' +
        '</div>' +
        '<span class="hint">可以手动输入绝对路径，也可以从本机磁盘中选择。</span></div>' +
        '<div class="form-error" id="pdError" hidden></div>' +
        '</div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn btn-primary" data-ok>添加</button></footer>' +
        '</div></div>';

      var input = root.querySelector('#pdPath');
      var errorBox = root.querySelector('#pdError');
      var backdrop = root.querySelector('.modal-backdrop');
      var done = false;

      function finish(value) {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(value);
      }
      function showError(msg) {
        errorBox.textContent = msg;
        errorBox.hidden = false;
      }

      root.querySelector('#pdBrowse').addEventListener('click', function () {
        dirPicker(input.value.trim()).then(function (dir) {
          if (dir) input.value = dir;
        });
      });
      root.querySelector('[data-ok]').addEventListener('click', function () {
        var path = input.value.trim();
        if (!path) {
          showError('项目目录不能为空');
          input.focus();
          return;
        }
        finish(path);
      });
      root.querySelector('[data-cancel]').addEventListener('click', function () { finish(null); });
      root.querySelector('[data-close]').addEventListener('click', function () { finish(null); });
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(null); });
      input.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); root.querySelector('[data-ok]').click(); }
        if (e.key === 'Escape') { e.preventDefault(); finish(null); }
      });
      input.focus();
    });
  }

  function dirPicker(startPath) {
    return new Promise(function (resolve) {
      var layer = document.createElement('div');
      layer.className = 'modal-backdrop picker-layer';
      layer.innerHTML = '<div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>选择项目目录</h3>' +
        '<p>浏览本机磁盘，或直接编辑路径后按回车。</p></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body picker-body">' +
        '<div class="path-row">' +
        '<input class="text-input" id="dpPath" autocomplete="off" placeholder="选择磁盘或输入绝对路径">' +
        '<button class="btn" id="dpGo">转到</button>' +
        '</div>' +
        '<div class="picker-tools"><button class="btn btn-sm" id="dpUp" disabled>上一级</button>' +
        '<button class="btn btn-sm" id="dpRoots">磁盘</button>' +
        '<span class="tiny" id="dpStatus">读取中…</span></div>' +
        '<div class="dir-list" id="dpList"></div>' +
        '<div class="form-error" id="dpError" hidden></div>' +
        '</div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn btn-primary" id="dpPick" disabled>选择此目录</button></footer>' +
        '</div>';

      document.body.appendChild(layer);

      var pathInput = layer.querySelector('#dpPath');
      var list = layer.querySelector('#dpList');
      var status = layer.querySelector('#dpStatus');
      var errorBox = layer.querySelector('#dpError');
      var upBtn = layer.querySelector('#dpUp');
      var pickBtn = layer.querySelector('#dpPick');
      var current = '';
      var parent = '';
      var done = false;

      function close(value) {
        if (done) return;
        done = true;
        layer.remove();
        resolve(value);
      }
      function showError(msg) {
        errorBox.textContent = msg;
        errorBox.hidden = false;
      }
      function load(path) {
        status.textContent = '读取中…';
        errorBox.hidden = true;
        api('/api/fs/dirs?path=' + encodeURIComponent(path)).then(function (data) {
          if (done) return;
          current = data.path || '';
          parent = data.parent || '';
          pathInput.value = current;
          upBtn.disabled = !current;
          pickBtn.disabled = !current;
          var dirs = data.dirs || [];
          status.textContent = current
            ? (dirs.length ? dirs.length + ' 个目录' : '没有子目录')
            : '选择磁盘';
          list.innerHTML = dirs.length
            ? dirs.map(function (d) {
              return '<button class="dir-item" data-dir="' + esc(d.path) + '">' +
                FOLDER_ICON + '<span>' + esc(d.name) + '</span></button>';
            }).join('')
            : '<div class="dir-empty">' + (current ? '这个目录下没有子目录' : '没有可用的磁盘') + '</div>';
        }).catch(function (e) {
          if (done) return;
          status.textContent = '';
          showError(e.message);
        });
      }

      list.addEventListener('click', function (e) {
        var item = e.target.closest('[data-dir]');
        if (item) load(item.dataset.dir);
      });
      upBtn.addEventListener('click', function () { if (current) load(parent || ''); });
      layer.querySelector('#dpRoots').addEventListener('click', function () { load(''); });
      layer.querySelector('#dpGo').addEventListener('click', function () {
        load(pathInput.value.trim());
      });
      pathInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); load(pathInput.value.trim()); }
      });
      pickBtn.addEventListener('click', function () { if (current) close(current); });
      layer.querySelector('[data-cancel]').addEventListener('click', function () { close(null); });
      layer.querySelector('[data-close]').addEventListener('click', function () { close(null); });
      layer.addEventListener('click', function (e) { if (e.target === layer) close(null); });

      load(startPath || '');
    });
  }

  /* ---------- 侧栏折叠 ---------- */

  function initSidebar() {
    state.projectsCollapsed = localStorage.getItem('sd.projectsCollapsed') === '1';
    state.foldersCollapsed = localStorage.getItem('sd.foldersCollapsed') === '1';
    state.sidebarCollapsed = localStorage.getItem('sd.sidebarCollapsed') === '1';
    applySidebarState();
    applySidebarShell();
  }

  function isNarrow() {
    return window.matchMedia('(max-width:1099px)').matches;
  }

  function applySidebarShell() {
    var app = $('.app');
    if (!app) return;
    app.classList.toggle('sidebar-collapsed', state.sidebarCollapsed);
    var btn = $('#btnToggleIndex');
    if (!btn) return;
    var label = state.sidebarCollapsed ? '展开索引' : '收起索引';
    btn.title = label;
    btn.setAttribute('aria-label', label);
    btn.setAttribute('aria-expanded', state.sidebarCollapsed ? 'false' : 'true');
  }

  function toggleSidebar() {
    state.sidebarCollapsed = !state.sidebarCollapsed;
    localStorage.setItem('sd.sidebarCollapsed', state.sidebarCollapsed ? '1' : '0');
    applySidebarShell();
  }

  function applySidebarState() {
    var sidebar = $('.sidebar');
    var projectsBlock = $('.side-projects');
    var foldersBlock = $('.sidebar .side-block.side-grow');
    if (!sidebar || !projectsBlock || !foldersBlock) return;

    projectsBlock.classList.toggle('is-collapsed', state.projectsCollapsed);
    foldersBlock.classList.toggle('is-collapsed', state.foldersCollapsed);
    sidebar.classList.toggle('projects-collapsed', state.projectsCollapsed);
    sidebar.classList.toggle('folders-collapsed', state.foldersCollapsed);
    $('#projectsHead').classList.toggle('is-collapsed', state.projectsCollapsed);
    $('#foldersHead').classList.toggle('is-collapsed', state.foldersCollapsed);

    localStorage.setItem('sd.projectsCollapsed', state.projectsCollapsed ? '1' : '0');
    localStorage.setItem('sd.foldersCollapsed', state.foldersCollapsed ? '1' : '0');

    if (state.projectsCollapsed) {
      var box = $('#projectSearch');
      if (box && !box.hidden) setProjectSearch(false);
    }
  }

  function toggleProjects() {
    state.projectsCollapsed = !state.projectsCollapsed;
    applySidebarState();
    if (!state.projectsCollapsed) animateTree($('#projectList'));
  }

  function toggleFolders() {
    state.foldersCollapsed = !state.foldersCollapsed;
    applySidebarState();
    if (!state.foldersCollapsed) animateTree($('#folderTree'));
  }

  /* ---------- 拖拽调整布局 ---------- */

  var LAYOUT_KEYS = { sidebar: 'sd.sidebarW', list: 'sd.listW', projects: 'sd.projectsH' };

  function clamp(v, min, max) {
    return Math.min(max, Math.max(min, v));
  }

  function initResizers() {
    var root = document.documentElement;
    var sidebar = $('.sidebar');
    var listPane = $('.list-pane');

    var sw = Number(localStorage.getItem(LAYOUT_KEYS.sidebar));
    if (sw) root.style.setProperty('--sidebar-w', clamp(sw, 180, 320) + 'px');
    var lw = Number(localStorage.getItem(LAYOUT_KEYS.list));
    if (lw) root.style.setProperty('--list-w', clamp(lw, 320, 560) + 'px');
    var ph = Number(localStorage.getItem(LAYOUT_KEYS.projects));
    if (ph) {
      root.style.setProperty('--projects-h', clamp(ph, 120, Math.max(200, sidebar.getBoundingClientRect().height - 240)) + 'px');
    }

    bindDrag($('[data-resize="sidebar"]'), 'x',
      function () { return sidebar.getBoundingClientRect().width; },
      function (v) {
        root.style.setProperty('--sidebar-w', v + 'px');
        localStorage.setItem(LAYOUT_KEYS.sidebar, Math.round(v));
      }, 180, 320);

    bindDrag($('[data-resize="list"]'), 'x',
      function () { return listPane.getBoundingClientRect().width; },
      function (v) {
        root.style.setProperty('--list-w', v + 'px');
        localStorage.setItem(LAYOUT_KEYS.list, Math.round(v));
      }, 320, 560);

    bindDrag($('#projectsSplitter'), 'y',
      function () { return $('.side-projects').getBoundingClientRect().height; },
      function (v) {
        root.style.setProperty('--projects-h', v + 'px');
        localStorage.setItem(LAYOUT_KEYS.projects, Math.round(v));
      }, 120, function () {
        return Math.max(200, sidebar.getBoundingClientRect().height - 240);
      });

    window.addEventListener('resize', function () {
      if (sidebar.getBoundingClientRect().width === 0) return;
      var current = parseFloat(getComputedStyle(root).getPropertyValue('--projects-h')) || 300;
      var maxH = Math.max(200, sidebar.getBoundingClientRect().height - 240);
      if (current > maxH) {
        root.style.setProperty('--projects-h', maxH + 'px');
        localStorage.setItem(LAYOUT_KEYS.projects, Math.round(maxH));
      }
    });
  }

  function bindDrag(el, axis, getSize, setSize, min, max) {
    if (!el) return;
    var active = false;
    var startPos = 0;
    var startSize = 0;

    function maxValue() {
      return typeof max === 'function' ? max() : max;
    }
    function apply(value) {
      setSize(clamp(value, min, maxValue()));
    }

    el.addEventListener('pointerdown', function (e) {
      if (e.button !== 0) return;
      e.preventDefault();
      active = true;
      startPos = axis === 'x' ? e.clientX : e.clientY;
      startSize = getSize();
      el.classList.add('is-active');
      document.body.classList.add('is-resizing', axis === 'x' ? 'is-col' : 'is-row');
      try { el.setPointerCapture(e.pointerId); } catch (err) { /* 忽略 */ }
    });

    el.addEventListener('pointermove', function (e) {
      if (!active) return;
      var current = axis === 'x' ? e.clientX : e.clientY;
      apply(startSize + (current - startPos));
    });

    function stop(e) {
      if (!active) return;
      active = false;
      el.classList.remove('is-active');
      document.body.classList.remove('is-resizing', 'is-col', 'is-row');
      try { el.releasePointerCapture(e.pointerId); } catch (err) { /* 忽略 */ }
    }
    el.addEventListener('pointerup', stop);
    el.addEventListener('pointercancel', stop);

    el.addEventListener('keydown', function (e) {
      var step = e.shiftKey ? 40 : 10;
      var delta = 0;
      if (axis === 'x' && e.key === 'ArrowLeft') delta = -step;
      if (axis === 'x' && e.key === 'ArrowRight') delta = step;
      if (axis === 'y' && e.key === 'ArrowUp') delta = -step;
      if (axis === 'y' && e.key === 'ArrowDown') delta = step;
      if (!delta) return;
      e.preventDefault();
      apply(getSize() + delta);
    });
  }

  /* ---------- 通用弹窗 ---------- */

  function promptDialog(opts) {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      var checkHtml = opts.checkboxLabel
        ? '<label class="check-line"><input type="checkbox" id="dlgCheck"' +
          (opts.checkboxChecked === false ? '' : ' checked') + '><span>' + esc(opts.checkboxLabel) + '</span></label>'
        : '';
      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>' + esc(opts.title) + '</h3></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body"><div class="field"><label for="dlgInput">' + esc(opts.label) + '</label>' +
        '<input class="text-input" id="dlgInput" autocomplete="off" value="' + esc(opts.value || '') + '" placeholder="' + esc(opts.placeholder || '') + '"></div>' +
        checkHtml + '</div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button><button class="btn btn-primary" data-ok>确定</button>' +
        '</footer></div></div>';

      var backdrop = root.querySelector('.modal-backdrop');
      var input = root.querySelector('#dlgInput');
      var done = false;
      function dialogValue() {
        var value = input.value.trim();
        if (!opts.checkboxLabel) return value;
        return { value: value, checked: root.querySelector('#dlgCheck').checked };
      }
      function finish(value) {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(value);
      }
      input.focus();
      input.select();
      root.querySelector('[data-ok]').addEventListener('click', function () { finish(dialogValue()); });
      root.querySelector('[data-cancel]').addEventListener('click', function () { finish(null); });
      root.querySelector('[data-close]').addEventListener('click', function () { finish(null); });
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(null); });
      input.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); finish(dialogValue()); }
        if (e.key === 'Escape') { e.preventDefault(); finish(null); }
      });
    });
  }

  /* ---------- 新建会话 ---------- */

  function folderOptions(folders, selectedID) {
    var items = '';
    function walk(parentID, depth) {
      (folders || []).filter(function (f) { return (f.parent_id || 0) === parentID; }).forEach(function (f) {
        var selected = selectedID != null && Number(selectedID) === f.id;
        items += '<option value="' + f.id + '"' + (selected ? ' selected' : '') + '>' +
          '　'.repeat(depth) + esc(f.name) + '</option>';
        walk(f.id, depth + 1);
      });
    }
    walk(0, 0);
    return '<option value="0"' + (!selectedID ? ' selected' : '') + '>待归类</option>' + items;
  }

  function newSessionDialog() {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      var initialFolderID = state.folderFilter.indexOf('f:') === 0 ? Number(state.folderFilter.slice(2)) || 0 : 0;
      var projectOptions = state.projects.map(function (p) {
        return '<option value="' + p.id + '"' + (p.id === state.projectId ? ' selected' : '') + '>' +
          esc(p.name) + ' — ' + esc(p.cwd) + '</option>';
      }).join('');

      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>新建会话</h3>' +
        '<p>会话会写入所选项目的 Claude Code 存储，并立即出现在CC会话管理。</p></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body">' +
        '<div class="field-row">' +
        '<div class="field"><label for="nsProject">项目</label><select id="nsProject">' + projectOptions + '</select></div>' +
        '<div class="field"><label for="nsFolder">归类</label><select id="nsFolder">' + folderOptions(state.folders, initialFolderID) + '</select></div>' +
        '</div>' +
        '<div class="field"><label for="nsTitle">标题（可选）</label>' +
        '<input class="text-input" id="nsTitle" autocomplete="off" placeholder="留空则由 Claude Code 自动生成"></div>' +
        '<div class="field"><label for="nsMessage">首条消息</label>' +
        '<textarea class="text-input" id="nsMessage" rows="4" placeholder="例如：帮我梳理这个模块的调用链，重点看鉴权入口。"></textarea></div>' +
        '<fieldset class="mode-set"><legend>创建方式</legend>' +
        '<label class="mode-item"><input type="radio" name="nsMode" value="cli" checked>' +
        '<span class="mode-body"><span class="mode-name">CLI 创建 <em class="rec">推荐</em></span>' +
        '<span class="mode-desc">调用 claude --session-id，约 2–4 秒，消耗少量 token。格式始终与当前版本一致。</span></span></label>' +
        '<label class="mode-item"><input type="radio" name="nsMode" value="draft">' +
        '<span class="mode-body"><span class="mode-name">仅写草稿</span>' +
        '<span class="mode-desc">不调用模型，直接生成可恢复的会话文件。零成本，但依赖内部 jsonl 格式。</span></span></label>' +
        '</fieldset>' +
        '<label class="check-line"><input type="checkbox" id="nsOpenAfter" checked>' +
        '<span>创建后在终端打开会话</span></label>' +
        '<div class="notice">草稿模式依赖 Claude Code 内部格式。版本升级后如果无法恢复，原始文件仍会保留，不会丢失数据。</div>' +
        '<div class="form-error" id="nsError" hidden></div>' +
        '</div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn btn-primary" id="nsCreate">创建会话</button></footer>' +
        '</div></div>';

      var backdrop = root.querySelector('.modal-backdrop');
      var projectSel = root.querySelector('#nsProject');
      var folderSel = root.querySelector('#nsFolder');
      var message = root.querySelector('#nsMessage');
      var titleInput = root.querySelector('#nsTitle');
      var createBtn = root.querySelector('#nsCreate');
      var errorBox = root.querySelector('#nsError');
      var openAfterBox = root.querySelector('#nsOpenAfter');
      var done = false;

      function finish(value) {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(value);
      }
      function showError(msg) {
        errorBox.textContent = msg;
        errorBox.hidden = false;
      }

      function reloadDialogFolders() {
        var pid = projectSel.value;
        api('/api/projects/' + pid + '/folders').then(function (data) {
          if (done || projectSel.value !== pid) return;
          folderSel.innerHTML = folderOptions(data.folders, Number(folderSel.value) || 0);
        }).catch(function () {});
      }

      projectSel.addEventListener('change', function () {
        folderSel.innerHTML = '<option value="0">待归类</option>';
        reloadDialogFolders();
      });
      reloadDialogFolders();

      createBtn.addEventListener('click', function () {
        if (createBtn.classList.contains('is-loading')) return;
        var firstMessage = message.value.trim();
        if (!firstMessage) {
          showError('首条消息不能为空');
          message.focus();
          return;
        }
        errorBox.hidden = true;
        var checked = root.querySelector('input[name="nsMode"]:checked');
        var payload = {
          project_id: Number(projectSel.value),
          folder_id: Number(folderSel.value) || 0,
          title: titleInput.value.trim(),
          first_message: firstMessage,
          mode: checked ? checked.value : 'cli'
        };
        createBtn.classList.add('is-loading');
        createBtn.disabled = true;
        createBtn.textContent = '创建中';
        api('/api/sessions', { method: 'POST', body: JSON.stringify(payload) }).then(function (sess) {
          finish({ session: sess, openAfter: openAfterBox && openAfterBox.checked });
        }).catch(function (e) {
          createBtn.classList.remove('is-loading');
          createBtn.disabled = false;
          createBtn.textContent = '创建会话';
          showError(e.message);
        });
      });

      root.querySelector('[data-cancel]').addEventListener('click', function () { finish(null); });
      root.querySelector('[data-close]').addEventListener('click', function () { finish(null); });
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(null); });
      message.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') finish(null);
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') createBtn.click();
      });
      message.focus();
    });
  }

  function createNewSession() {
    newSessionDialog().then(function (result) {
      if (!result) return;
      var sess = result.session;
      toast('会话已创建');
      var id = sess.id;
      var pid = sess.project_id || state.projectId;
      return loadProjects().then(function () {
        if (pid && pid !== state.projectId) {
          return selectProject(pid, true);
        }
        return Promise.all([loadFolders(), loadSessions(true)]);
      }).then(function () {
        if (id) return selectSession(id);
      }).then(function () {
        if (!result.openAfter || !id) return;
        return api('/api/sessions/' + encodeURIComponent(id) + '/open', { method: 'POST' }).then(function (res) {
          var term = res.result && res.result.terminal === 'windows-terminal' ? 'Windows Terminal' : '新终端窗口';
          toast('已在 ' + term + ' 打开');
        }).catch(function (e) {
          toast('会话已创建，但终端打开失败：' + e.message, 'error');
        });
      });
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  /* ---------- 多选删除 ---------- */

  function resetSelection() {
    state.selectMode = false;
    state.selected = {};
    document.body.classList.remove('is-selecting');
    var bar = $('#bulkBar');
    var btn = $('#btnSelectMode');
    if (bar) bar.hidden = true;
    if (btn) btn.textContent = '多选';
  }

  function setSelectMode(on) {
    state.selectMode = on;
    if (!on) state.selected = {};
    document.body.classList.toggle('is-selecting', on);
    $('#bulkBar').hidden = !on;
    $('#btnSelectMode').textContent = on ? '退出多选' : '多选';
    updateBulkBar();
    renderSessions();
  }

  function updateBulkBar() {
    var count = Object.keys(state.selected).length;
    var bar = $('#bulkBar');
    if (!bar) return;
    $('#bulkCount').textContent = '已选 ' + count + ' 项';
    var move = $('#bulkMove');
    if (move) move.hidden = state.folderFilter === 'trash';
    var del = $('#bulkDelete');
    del.disabled = count === 0;
    del.textContent = state.folderFilter === 'trash' ? '彻底删除' : '删除';
    if (move) move.disabled = count === 0;
  }

  function toggleSelect(id) {
    if (state.selected[id]) delete state.selected[id];
    else state.selected[id] = true;
    var item = $('#listBody').querySelector('[data-session="' + id + '"]');
    if (item) {
      item.classList.toggle('is-selected', !!state.selected[id]);
      var check = item.querySelector('.s-check');
      if (check) {
        check.classList.toggle('is-checked', !!state.selected[id]);
        check.setAttribute('aria-checked', String(!!state.selected[id]));
      }
    }
    updateBulkBar();
  }

  function selectAllSessions() {
    state.selected = {};
    state.sessions.forEach(function (s) { state.selected[s.id] = true; });
    renderSessions();
    updateBulkBar();
  }

  function clearSelection() {
    state.selected = {};
    renderSessions();
    updateBulkBar();
  }

  function assignFolderToSessions(ids, folderID) {
    return ids.reduce(function (chain, id) {
      return chain.then(function () {
        return api('/api/sessions/' + encodeURIComponent(id), {
          method: 'PATCH',
          body: JSON.stringify({ folder_id: folderID })
        });
      });
    }, Promise.resolve());
  }

  function moveSessionsToFolder(ids, folderID, doneMessage) {
    return assignFolderToSessions(ids, folderID).then(function () {
      toast(doneMessage);
      if (state.detail && ids.indexOf(state.detail.session.id) !== -1) {
        state.detail.session.folder_id = folderID || null;
      }
      return Promise.all([loadFolders(), loadSessions()]);
    });
  }

  function singleMoveSession(id) {
    var s = state.sessions.filter(function (x) { return x.id === id; })[0];
    folderPickerDialog('加入归类', '归类只影响CC会话管理的索引，不移动磁盘上的会话文件。', s ? s.folder_id : 0)
      .then(function (folderID) {
        if (folderID === null) return;
        return moveSessionsToFolder([id], folderID, folderID ? '已加入归类' : '已移到待归类');
      }).catch(function (e) {
        toast(e.message, 'error');
      });
  }

  function openSessionById(id) {
    api('/api/sessions/' + encodeURIComponent(id) + '/open', { method: 'POST' }).then(function (res) {
      var term = res.result && res.result.terminal === 'windows-terminal' ? 'Windows Terminal' : '新终端窗口';
      toast('已在 ' + term + ' 打开');
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  function compactSessionById(id) {
    toast('正在压缩上下文…');
    api('/api/sessions/' + encodeURIComponent(id) + '/compact', { method: 'POST' })
      .then(function () {
        toast('上下文已压缩');
        return loadSessions(true).then(function () {
          if (state.sessionId === id) return selectSession(id);
        });
      }).catch(function (e) {
        toast(e.message, 'error');
      });
  }

  function copySessionById(id, mode) {
    var s = state.sessions.filter(function (x) { return x.id === id; })[0];
    var isBranch = mode === 'branch';
    promptDialog({
      title: isBranch ? '创建分支' : '复制为副本',
      label: isBranch ? '分支名称' : '副本名称',
      value: '',
      placeholder: '留空自动命名「' + (s ? s.title : '会话') + '-' + mode + '-001」，填写则直接使用输入名称',
      checkboxLabel: '创建后在终端打开会话',
      checkboxChecked: true
    }).then(function (result) {
      if (result === null) return;
      return api('/api/sessions/' + encodeURIComponent(id) + '/' + mode, {
        method: 'POST',
        body: JSON.stringify({ name: result.value })
      }).then(function (sess) {
        toast(isBranch ? '已创建分支会话' : '已创建副本会话');
        return loadProjects().then(function () {
          return Promise.all([loadFolders(), loadSessions(true)]);
        }).then(function () {
          if (isBranch && sess && sess.id) return selectSession(sess.id);
        }).then(function () {
          if (!result.checked || !sess || !sess.id) return;
          return api('/api/sessions/' + encodeURIComponent(sess.id) + '/open', { method: 'POST' }).then(function (res) {
            var term = res.result && res.result.terminal === 'windows-terminal' ? 'Windows Terminal' : '新终端窗口';
            toast('已在 ' + term + ' 打开');
          }).catch(function (e) {
            toast('会话已创建，但终端打开失败：' + e.message, 'error');
          });
        });
      });
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  function renameSessionById(id) {
    var s = state.sessions.filter(function (x) { return x.id === id; })[0];
    if (!s) return;
    promptDialog({ title: '重命名会话', label: '标题', value: s.title }).then(function (title) {
      if (title === null) return;
      if (!title) {
        toast('标题不能为空', 'error');
        return;
      }
      api('/api/sessions/' + encodeURIComponent(id), {
        method: 'PATCH',
        body: JSON.stringify({ title: title })
      }).then(function () {
        toast('标题已更新');
        return loadSessions();
      }).then(function () {
        if (state.sessionId === id) return selectSession(id);
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function deleteSessionById(id) {
    var target = state.sessions.filter(function (x) { return x.id === id; })[0];
    confirmDialog({
      title: '删除会话',
      message: '「' + (target ? target.title : '这个会话') + '」会移到本机回收站，之后可以从回收站还原。',
      confirmText: '移到回收站',
      danger: true
    }).then(function (ok) {
      if (!ok) return;
      api('/api/sessions/' + encodeURIComponent(id), { method: 'DELETE' }).then(function () {
        toast('已移到回收站');
        if (state.sessionId === id) {
          state.sessionId = null;
          clearDetail();
        }
        return loadSessions(true).then(loadProjects);
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function moveSessionProject(id) {
    var s = state.sessions.filter(function (x) { return x.id === id; })[0];
    if (!s) return;
    projectPickerDialog(s).then(function (pid) {
      if (pid === null) return;
      var current = s.display_project_id || s.project_id;
      if (pid === current) return;
      api('/api/sessions/' + encodeURIComponent(id), {
        method: 'PATCH',
        body: JSON.stringify({ project_id: pid })
      }).then(function () {
        toast('已归属到项目');
        return loadProjects();
      }).then(function () {
        if (state.sessionId === id) {
          return selectProject(pid, true).then(function () { return selectSession(id); });
        }
        return loadSessions(true);
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function projectPickerDialog(s) {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      var current = s.display_project_id || s.project_id;
      var options = state.projects.map(function (p) {
        return '<option value="' + p.id + '"' + (p.id === current ? ' selected' : '') + '>' +
          esc(p.name) + ' — ' + esc(p.cwd) + '</option>';
      }).join('');
      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>归属项目</h3>' +
        '<p>手动选择会锁定归属；会话可在所选项目下浏览。</p></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body"><div class="field"><label for="ppProject">项目</label>' +
        '<select id="ppProject">' + options + '</select></div></div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn btn-primary" data-ok>确定</button></footer>' +
        '</div></div>';

      var select = root.querySelector('#ppProject');
      var backdrop = root.querySelector('.modal-backdrop');
      var done = false;

      function finish(value) {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(value);
      }

      root.querySelector('[data-ok]').addEventListener('click', function () {
        finish(Number(select.value) || null);
      });
      root.querySelector('[data-cancel]').addEventListener('click', function () { finish(null); });
      root.querySelector('[data-close]').addEventListener('click', function () { finish(null); });
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(null); });
      select.focus();
    });
  }

  function bulkMoveSessions() {
    var ids = Object.keys(state.selected);
    if (!ids.length) return;
    folderPickerDialog('批量加入归类', '将选中的 ' + ids.length + ' 个会话移入所选归类。', 0)
      .then(function (folderID) {
        if (folderID === null) return;
        return moveSessionsToFolder(ids, folderID, folderID ? '已加入归类' : '已移到待归类')
          .then(function () { clearSelection(); });
      }).catch(function (e) {
        toast(e.message, 'error');
      });
  }

  function emptyTrash() {
    if (!state.projectId) return;
    confirmDialog({
      title: '清空回收站',
      message: '当前项目回收站中的会话文件会被永久删除，无法还原。',
      confirmText: '清空回收站',
      danger: true
    }).then(function (ok) {
      if (!ok) return;
      api('/api/projects/' + state.projectId + '/trash/purge', { method: 'POST' }).then(function (res) {
        var n = res && res.purged ? res.purged : 0;
        toast(n ? '回收站已清空（' + n + ' 个会话）' : '回收站已清空');
        state.sessionId = null;
        clearDetail();
        return loadSessions(true).then(loadProjects);
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function bulkDelete() {
    var ids = Object.keys(state.selected);
    if (!ids.length) return;
    var permanent = state.folderFilter === 'trash';
    confirmDialog({
      title: permanent ? '彻底删除' : '删除会话',
      message: permanent
        ? '选中的 ' + ids.length + ' 个会话会被永久删除，无法恢复。'
        : '选中的 ' + ids.length + ' 个会话会移到回收站，可以还原。',
      confirmText: permanent ? '彻底删除' : '移到回收站',
      danger: true
    }).then(function (ok) {
      if (!ok) return;
      api('/api/sessions/batch-delete', {
        method: 'POST',
        body: JSON.stringify({ ids: ids, permanent: permanent })
      }).then(function (res) {
        var failed = res.failed || [];
        toast('已删除 ' + res.deleted + ' 个会话' + (failed.length ? '，' + failed.length + ' 个失败' : ''), failed.length ? 'error' : 'ok');
        state.selectMode = false;
        state.selected = {};
        document.body.classList.remove('is-selecting');
        $('#bulkBar').hidden = true;
        $('#btnSelectMode').textContent = '多选';
        updateBulkBar();
        return loadSessions(true).then(loadProjects);
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  /* ---------- 删除与回收站 ---------- */

  // 只有「知道了」的提示弹窗，用于展示无法继续的原因。
  function infoDialog(opts) {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>' + esc(opts.title) + '</h3></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body"><p class="dialog-text">' + esc(opts.message) + '</p></div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn btn-primary" data-ok>知道了</button></footer>' +
        '</div></div>';
      var backdrop = root.querySelector('.modal-backdrop');
      var done = false;
      function finish() {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(true);
      }
      root.querySelector('[data-ok]').addEventListener('click', finish);
      root.querySelector('[data-close]').addEventListener('click', finish);
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(); });
    });
  }

  function confirmDialog(opts) {
    return new Promise(function (resolve) {
      var root = $('#modalRoot');
      root.innerHTML = '<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true">' +
        '<header class="modal-head"><div><h3>' + esc(opts.title) + '</h3></div>' +
        '<button class="icon-btn" data-close aria-label="关闭">' +
        '<svg class="ico" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"></path></svg></button></header>' +
        '<div class="modal-body"><p class="dialog-text">' + esc(opts.message) + '</p></div>' +
        '<footer class="modal-foot"><span class="grow"></span>' +
        '<button class="btn" data-cancel>取消</button>' +
        '<button class="btn ' + (opts.danger ? 'btn-danger' : 'btn-primary') + '" data-ok>' +
        esc(opts.confirmText || '确定') + '</button></footer>' +
        '</div></div>';

      var backdrop = root.querySelector('.modal-backdrop');
      var done = false;
      function finish(value) {
        if (done) return;
        done = true;
        root.innerHTML = '';
        resolve(value);
      }
      root.querySelector('[data-ok]').addEventListener('click', function () { finish(true); });
      root.querySelector('[data-cancel]').addEventListener('click', function () { finish(false); });
      root.querySelector('[data-close]').addEventListener('click', function () { finish(false); });
      backdrop.addEventListener('click', function (e) { if (e.target === backdrop) finish(false); });
    });
  }

  function deleteCurrent() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    confirmDialog({
      title: '删除会话',
      message: '「' + s.title + '」会移到本机回收站，之后可以从回收站还原。',
      confirmText: '移到回收站',
      danger: true
    }).then(function (ok) {
      if (!ok) return;
      api('/api/sessions/' + encodeURIComponent(s.id), { method: 'DELETE' }).then(function () {
        toast('已移到回收站');
        state.sessionId = null;
        clearDetail();
        return loadSessions(true).then(loadProjects);
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  function restoreCurrent() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    api('/api/sessions/' + encodeURIComponent(s.id) + '/restore', { method: 'POST' }).then(function () {
      toast('会话已还原');
      state.folderFilter = 'all';
      rememberFilter(state.projectId, 'all');
      renderFolders();
      renderFolderTabs();
      return loadSessions(true).then(function () {
        return selectSession(s.id);
      }).then(loadProjects);
    }).catch(function (e) {
      toast(e.message, 'error');
    });
  }

  function purgeCurrent() {
    var s = state.detail && state.detail.session;
    if (!s) return;
    confirmDialog({
      title: '彻底删除',
      message: '回收站中的会话文件会被永久删除，无法还原。',
      confirmText: '彻底删除',
      danger: true
    }).then(function (ok) {
      if (!ok) return;
      api('/api/sessions/' + encodeURIComponent(s.id) + '?permanent=1', { method: 'DELETE' }).then(function () {
        toast('已彻底删除');
        state.sessionId = null;
        clearDetail();
        return loadSessions(true).then(loadProjects);
      }).catch(function (e) {
        toast(e.message, 'error');
      });
    });
  }

  /* ---------- 实时刷新 ---------- */

  function initEvents() {
    if (!window.EventSource) return;
    if (new URLSearchParams(location.search).get('noevents') === '1') return;
    var source = new EventSource('/api/events');
    var timer = null;
    var aiTimer = null;
    var aiPollTimer = null;
    var aiRefreshPending = false;

    function refreshAIView() {
      if (document.hidden) {
        aiRefreshPending = true;
        return;
      }
      var scroll = $('#listBody').scrollTop;
      loadSessions(true).then(function () {
        $('#listBody').scrollTop = scroll;
      }).catch(function () {});
      // 概要生成完成时刷新详情卡片，保留阅读位置。
      if (!state.sessionId) return;
      var threadScroll = $('#thread').scrollTop;
      api('/api/sessions/' + encodeURIComponent(state.sessionId)).then(function (detail) {
        if (!detail || state.sessionId !== detail.session.id) return;
        state.detail = detail;
        renderDetail(detail);
        $('#thread').scrollTop = threadScroll;
      }).catch(function () {});
    }

    // SSE 事件之外再做短轮询，直到任务结束。
    // 避免最后一个事件被防抖合并或连接抖动时，列表徽章停在半完成状态。
    function pollAIUntilDone() {
      if (aiPollTimer) return;
      var started = Date.now();
      function tick() {
        var query = state.projectId ? '?project_id=' + state.projectId : '';
        api('/api/ai/status' + query).then(function (st) {
          state.aiStatus = st;
          refreshAIView();
          var job = st.job;
          var running = !!(job && (job.status === 'queued' || job.status === 'running'));
          if (running && Date.now() - started < 10 * 60 * 1000) {
            aiPollTimer = setTimeout(tick, 2500);
          } else {
            aiPollTimer = null;
            // 任务结束后补一次，确保最后写入的会话也显示徽章。
            setTimeout(refreshAIView, 1000);
          }
        }).catch(function () {
          aiPollTimer = null;
        });
      }
      tick();
    }

    source.addEventListener('ai', function () {
      clearTimeout(aiTimer);
      aiTimer = setTimeout(function () {
        refreshAIView();
        pollAIUntilDone();
      }, 800);
    });

    document.addEventListener('visibilitychange', function () {
      if (document.hidden || !aiRefreshPending) return;
      aiRefreshPending = false;
      refreshAIView();
    });
    source.addEventListener('scan', function () {
      clearTimeout(timer);
      timer = setTimeout(function () {
        if (document.hidden) return;
        var scroll = $('#listBody').scrollTop;
        loadProjects().then(function () {
          if (!state.projectId) return null;
          return Promise.all([loadFolders(), loadSessions(true)]);
        }).then(function () {
          $('#listBody').scrollTop = scroll;
        }).catch(function () {});
      }, 400);
    });
  }

  /* ---------- 格式化 ---------- */

  function parseDate(iso) {
    var t = Date.parse(iso || '');
    return isNaN(t) ? null : new Date(t);
  }
  function fmtClock(iso) {
    var d = parseDate(iso);
    return d ? d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }) : '';
  }
  function fmtDay(iso) {
    var d = parseDate(iso);
    return d ? d.toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' }) : '';
  }
  function dayKey(iso) {
    var d = parseDate(iso);
    return d ? (d.getFullYear() + '-' + d.getMonth() + '-' + d.getDate()) : '';
  }
  function listTimeParts(iso) {
    var d = parseDate(iso);
    if (!d) return { date: '', time: '' };
    var p = function (n) { return String(n).padStart(2, '0'); };
    return {
      date: p(d.getMonth() + 1) + '-' + p(d.getDate()),
      time: p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds())
    };
  }
  function fmtSize(n) {
    if (!n) return '0 KB';
    if (n < 1024 * 1024) return Math.round(n / 1024) + ' KB';
    return (n / 1048576).toFixed(1) + ' MB';
  }
  function isRecent(iso) {
    var d = parseDate(iso);
    return d ? (Date.now() - d.getTime() < 3600 * 1000) : false;
  }

  /* ---------- 初始化 ---------- */

  function bindStatic() {
    var themeBtn = $('#btnTheme');
    function applyTheme(theme) {
      document.documentElement.dataset.theme = theme;
      document.querySelector('meta[name="theme-color"]').content = theme === 'dark' ? '#0E1311' : '#F4F5F0';
      // 图标提示切换目标：浅色显示月亮，深色显示太阳。
      themeBtn.innerHTML = theme === 'dark'
        ? '<svg class="ico" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="3"></circle><path d="M8 1.5v1.6M8 12.9v1.6M1.5 8h1.6M12.9 8h1.6M3.4 3.4l1.1 1.1M11.5 11.5l1.1 1.1M12.6 3.4l-1.1 1.1M4.5 11.5l-1.1 1.1"></path></svg>'
        : '<svg class="ico" viewBox="0 0 16 16" aria-hidden="true"><path d="M13.3 9.7A5.6 5.6 0 0 1 6.3 2.7a5.6 5.6 0 1 0 7 7Z"></path></svg>';
      var label = theme === 'dark' ? '切换到浅色主题' : '切换到深色主题';
      themeBtn.title = label;
      themeBtn.setAttribute('aria-label', label);
    }
    applyTheme(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
    themeBtn.addEventListener('click', function () {
      var theme = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
      applyTheme(theme);
      try { localStorage.setItem('sd.theme', theme); } catch (e) { /* 隐私模式下仍可在当前页面切换 */ }
    });

    $('#projectList').addEventListener('click', function (e) {
      var addEmpty = e.target.closest('[data-add-project-empty]');
      if (addEmpty) {
        addProject();
        return;
      }
      var up = e.target.closest('[data-project-up]');
      if (up) {
        e.stopPropagation();
        movePinnedProject(Number(up.dataset.projectUp), 'up');
        return;
      }
      var down = e.target.closest('[data-project-down]');
      if (down) {
        e.stopPropagation();
        movePinnedProject(Number(down.dataset.projectDown), 'down');
        return;
      }
      var pin = e.target.closest('[data-pin-project]');
      if (pin) {
        e.stopPropagation();
        toggleProjectPin(Number(pin.dataset.pinProject), pin.classList.contains('is-on'));
        return;
      }
      var aliasBtn = e.target.closest('[data-alias-project]');
      if (aliasBtn) {
        e.stopPropagation();
        editProjectAlias(Number(aliasBtn.dataset.aliasProject));
        return;
      }
      var del = e.target.closest('[data-del-project]');
      if (del) {
        e.stopPropagation();
        deleteProject(Number(del.dataset.delProject));
        return;
      }
      var btn = e.target.closest('[data-project]');
      if (btn) selectProject(Number(btn.dataset.project), true);
    });

    $('#folderTree').addEventListener('click', function (e) {
      var clearFolderSearch = e.target.closest('[data-clear-folder-search]');
      if (clearFolderSearch) {
        setFolderSearch(false);
        return;
      }
      var fres = e.target.closest('[data-folder-resume]');
      if (fres) {
        e.stopPropagation();
        resumeFolderLatest(Number(fres.dataset.folderResume));
        return;
      }
      var fup = e.target.closest('[data-folder-up]');
      if (fup) {
        e.stopPropagation();
        moveFolder(Number(fup.dataset.folderUp), 'up');
        return;
      }
      var fdown = e.target.closest('[data-folder-down]');
      if (fdown) {
        e.stopPropagation();
        moveFolder(Number(fdown.dataset.folderDown), 'down');
        return;
      }
      var del = e.target.closest('[data-del-folder]');
      if (del) {
        e.stopPropagation();
        deleteFolder(Number(del.dataset.delFolder));
        return;
      }
      var ren = e.target.closest('[data-rename-folder]');
      if (ren) {
        e.stopPropagation();
        renameFolder(Number(ren.dataset.renameFolder));
        return;
      }
      var add = e.target.closest('[data-add-folder]');
      if (add) {
        e.stopPropagation();
        addFolder(Number(add.dataset.addFolder));
        return;
      }
      var cat = e.target.closest('[data-toggle-categorized]');
      if (cat) {
        if (state.folderFilter === 'categorized') {
          state.expanded.categorized = !state.expanded.categorized;
          renderFolders();
        } else {
          state.expanded.categorized = true;
          setFolderFilter('categorized');
        }
        animateTree($('#folderTree'));
        return;
      }
      var btn = e.target.closest('[data-filter]');
      if (!btn) return;
      var fid = btn.dataset.folderId;
      if (fid) {
        var id = Number(fid);
        var hasKids = btn.dataset.hasKids === '1';
        var active = state.folderFilter === 'f:' + id;
        if (hasKids) {
          state.expanded[id] = active ? !state.expanded[id] : true;
        }
        setFolderFilter('f:' + id);
        if (hasKids) animateTree($('#folderTree'));
      } else {
        setFolderFilter(btn.dataset.filter);
      }
    });

    updateViewSwitch();
    var sortSel = $('#sortSelect');
    sortSel.value = state.sortMode === 'messages' || state.sortMode === 'title' ? state.sortMode : 'recent';
    state.sortMode = sortSel.value;
    sortSel.addEventListener('change', function () {
      state.sortMode = sortSel.value;
      localStorage.setItem('sd.sortMode', state.sortMode);
      loadSessions(true);
    });
    $('#listCrumb').addEventListener('click', function (e) {
      var more = e.target.closest('[data-crumb-more]');
      if (more) {
        state.crumbExpanded = true;
        renderCrumb();
        return;
      }
      var btn = e.target.closest('[data-crumb]');
      if (btn) setFolderFilter(btn.dataset.crumb);
    });

    $('#listSub').addEventListener('click', function (e) {
      var name = e.target.closest('#projectName');
      if (!name) return;
      e.preventDefault();
      copyProjectPath(name);
    });

    $('#viewSwitch').addEventListener('click', function (e) {
      var btn = e.target.closest('button[data-view]');
      if (btn && !btn.disabled) setViewMode(btn.dataset.view);
    });

    $('#detailHead').addEventListener('click', function (e) {
      var link = e.target.closest('[data-lineage]');
      if (link) selectSession(link.dataset.lineage);
    });

    $('#listBody').addEventListener('click', function (e) {
      var treeToggle = e.target.closest('[data-tree-toggle]');
      if (treeToggle) {
        e.preventDefault();
        e.stopPropagation();
        toggleTree(treeToggle.dataset.treeToggle);
        return;
      }
      var gotoSession = e.target.closest('[data-goto-session]');
      if (gotoSession) {
        e.preventDefault();
        e.stopPropagation();
        var parentId = gotoSession.dataset.gotoSession;
        selectSession(parentId);
        var parentRow = $('#listBody [data-session="' + parentId + '"]');
        if (parentRow) parentRow.scrollIntoView({ block: 'center' });
        return;
      }
      var family = e.target.closest('[data-family]');
      if (family) {
        e.preventDefault();
        e.stopPropagation();
        setViewMode('tree');
        selectSession(family.dataset.family);
        return;
      }
      var applyTitle = e.target.closest('[data-apply-title]');
      if (applyTitle) {
        e.preventDefault();
        e.stopPropagation();
        applySuggestedTitle(applyTitle.dataset.applyTitle, applyTitle.dataset.title);
        return;
      }
      var act = e.target.closest('[data-move-folder],[data-session-open],[data-session-rename],[data-session-compact],[data-session-branch],[data-session-fork],[data-session-project],[data-session-delete]');
      if (act) {
        e.preventDefault();
        if (act.dataset.moveFolder) singleMoveSession(act.dataset.moveFolder);
        else if (act.dataset.sessionOpen) openSessionById(act.dataset.sessionOpen);
        else if (act.dataset.sessionRename) renameSessionById(act.dataset.sessionRename);
        else if (act.dataset.sessionCompact) compactSessionById(act.dataset.sessionCompact);
        else if (act.dataset.sessionBranch) copySessionById(act.dataset.sessionBranch, 'branch');
        else if (act.dataset.sessionFork) copySessionById(act.dataset.sessionFork, 'fork');
        else if (act.dataset.sessionProject) moveSessionProject(act.dataset.sessionProject);
        else if (act.dataset.sessionDelete) deleteSessionById(act.dataset.sessionDelete);
        return;
      }
      var item = e.target.closest('[data-session]');
      if (item) {
        if (state.selectMode) { toggleSelect(item.dataset.session); return; }
        selectSession(item.dataset.session);
        return;
      }
      var action = e.target.closest('[data-action]');
      if (!action) return;
      if (action.dataset.action === 'retry') loadSessions();
      if (action.dataset.action === 'sync') doSync();
      if (action.dataset.action === 'reset') setFolderFilter('all');
      if (action.dataset.action === 'clear-search') {
        state.query = '';
        $('#search').value = '';
        loadSessions();
      }
    });

    $('#thread').addEventListener('click', function (e) {
      if (e.target.closest('[data-action="clear-search"]')) {
        state.query = '';
        $('#search').value = '';
        loadSessions();
        return;
      }
      if (e.target.closest('[data-action="reset"]')) {
        setFolderFilter('all');
      }
    });

    $('#btnSync').addEventListener('click', doSync);
    $('#btnAITidy').addEventListener('click', aiTidy);
    $('#btnAISettings').addEventListener('click', aiSettingsDialog);
    $('#btnToggleIndex').addEventListener('click', function (e) {
      e.stopPropagation();
      if (isNarrow()) {
        $('.sidebar').classList.toggle('is-open');
        return;
      }
      toggleSidebar();
    });
    document.addEventListener('click', function (e) {
      var sidebar = $('.sidebar');
      if (!sidebar || !sidebar.classList.contains('is-open')) return;
      if (e.target.closest('.sidebar') || e.target.closest('#btnToggleIndex')) return;
      sidebar.classList.remove('is-open');
    });
    $('#projectsHead').addEventListener('click', function (e) {
      if (e.target.closest('.side-head-actions')) return;
      toggleProjects();
    });
    $('#foldersHead').addEventListener('click', function (e) {
      if (e.target.closest('.side-head-actions')) return;
      toggleFolders();
    });
    $('#btnAddFolder').addEventListener('click', function () {
      if (state.foldersCollapsed) {
        state.foldersCollapsed = false;
        applySidebarState();
      }
      addFolder(0);
    });
    $('#btnAddProject').addEventListener('click', function () {
      if (state.projectsCollapsed) {
        state.projectsCollapsed = false;
        applySidebarState();
      }
      addProject();
    });
    $('#btnProjectSearch').addEventListener('click', function () {
      if (state.projectsCollapsed) {
        state.projectsCollapsed = false;
        applySidebarState();
      }
      setProjectSearch($('#projectSearch').hidden);
    });
    $('#btnFolderSearch').addEventListener('click', function () {
      setFolderSearch($('#folderSearch').hidden);
    });
    $('#folderQuery').addEventListener('input', function (e) {
      state.folderQuery = e.target.value;
      renderFolders();
    });
    $('#folderQuery').addEventListener('keydown', function (e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        setFolderSearch(false);
      }
    });
    $('#projectQuery').addEventListener('input', function (e) {
      state.projectQuery = e.target.value;
      renderProjects();
    });
    $('#projectQuery').addEventListener('keydown', function (e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        setProjectSearch(false);
      }
    });
    $('#btnNewSession').addEventListener('click', createNewSession);
    $('#btnSelectMode').addEventListener('click', function () { setSelectMode(!state.selectMode); });
    $('#bulkBar').addEventListener('click', function (e) {
      var action = e.target.closest('[data-bulk]');
      if (!action) return;
      if (action.dataset.bulk === 'all') selectAllSessions();
      if (action.dataset.bulk === 'none') clearSelection();
      if (action.dataset.bulk === 'exit') setSelectMode(false);
    });
    $('#bulkMove').addEventListener('click', bulkMoveSessions);
    $('#bulkDelete').addEventListener('click', bulkDelete);
    $('#btnEmptyTrash').addEventListener('click', emptyTrash);

    $('#search').addEventListener('input', function (e) {
      clearTimeout(searchTimer);
      var value = e.target.value;
      searchTimer = setTimeout(function () {
        state.query = value.trim();
        resetSelection();
        loadSessions();
      }, 250);
    });

    document.addEventListener('keydown', function (e) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        $('#search').focus();
        $('#search').select();
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'p') {
        e.preventDefault();
        setProjectSearch(true);
      }
    });
  }

  function applyHash() {
    var m = initialHash.match(/^#session=([0-9a-fA-F-]{36})$/);
    if (m) selectSession(m[1]);
  }

  function applyQuery() {
    var params = new URLSearchParams(location.search);
    var filter = params.get('filter');
    if (filter === 'trash' || filter === 'unfiled' || filter === 'all') {
      state.folderFilter = filter;
      if (!initialHash) {
        state.sessionId = null;
        clearDetail();
      }
      renderFolders();
      renderFolderTabs();
      loadSessions(true);
    }
    if (params.get('new') === '1') {
      setTimeout(createNewSession, 0);
    }
  }

  function folderFilterValid(filter) {
    if (filter === 'all' || filter === 'categorized' || filter === 'unfiled' || filter === 'trash') return true;
    if (filter.indexOf('f:') === 0) {
      var fid = Number(filter.slice(2));
      return state.folders.some(function (f) { return f.id === fid; });
    }
    return false;
  }

  function restoreLastState() {
    // 归类已由 selectProject 按项目记忆恢复，这里只补回上次打开的会话。
    if (initialHash) return Promise.resolve();
    if (!savedLast.session || state.sessionId === savedLast.session) return Promise.resolve();
    var exists = state.sessions.some(function (s) { return s.id === savedLast.session; });
    if (exists) return selectSession(savedLast.session);
    return Promise.resolve();
  }

  function init() {
    bindStatic();
    initResizers();
    initSidebar();
    initEvents();
    clearDetail();
    renderListState('loading');
    loadProjects().then(function () {
      return restoreLastState();
    }).then(function () {
      applyHash();
      applyQuery();
      return refreshStats();
    }).catch(function (e) {
      renderListState('error', e.message);
      setSync('加载失败', true);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
