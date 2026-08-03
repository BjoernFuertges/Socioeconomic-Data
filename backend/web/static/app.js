(function () {
  "use strict";

  var config = window.APP_CONFIG || { apiBaseUrl: "http://localhost:8081/api/v1", docsUrl: "" };
  var API_BASE = config.apiBaseUrl.replace(/\/+$/, "");

  var els = {
    apiStatus: document.getElementById("api-status"),
    apiBaseLabel: document.getElementById("api-base-label"),
    docsLink: document.getElementById("docs-link"),
    navTabs: document.querySelectorAll(".nav-tab"),
    views: {
      nuts: document.getElementById("view-nuts"),
      lau: document.getElementById("view-lau"),
    },
    nutsSearch: document.getElementById("nuts-search"),
    nutsList: document.getElementById("nuts-list"),
    nutsDetail: document.getElementById("nuts-detail"),
    lauSearch: document.getElementById("lau-search"),
    lauList: document.getElementById("lau-list"),
    lauDetail: document.getElementById("lau-detail"),
    tooltip: document.getElementById("tooltip"),
  };

  els.apiBaseLabel.textContent = API_BASE;
  if (config.docsUrl) {
    els.docsLink.href = config.docsUrl;
    els.docsLink.hidden = false;
  }

  // ---- API helpers -------------------------------------------------------

  function apiGet(path, params) {
    var url = API_BASE + path;
    if (params) {
      var query = Object.keys(params)
        .filter(function (key) {
          return params[key] !== undefined && params[key] !== null && params[key] !== "";
        })
        .map(function (key) {
          return encodeURIComponent(key) + "=" + encodeURIComponent(params[key]);
        })
        .join("&");
      if (query) url += "?" + query;
    }
    return fetch(url).then(function (res) {
      return res
        .json()
        .catch(function () {
          return { success: false, error: "Ungültige Antwort (HTTP " + res.status + ")" };
        })
        .then(function (body) {
          if (!res.ok || !body || body.success === false) {
            var message = (body && body.error) || "HTTP " + res.status;
            throw new Error(message);
          }
          return body;
        });
    });
  }

  // The Go API serializes some nullable fields (LAU metadata) as
  // database/sql.Null* structs ({"Int64":1,"Valid":true}) instead of plain
  // scalars, since those types have no custom JSON marshaler. Unwrap both
  // that shape and plain values so the UI works either way.
  function unwrapNull(field) {
    if (field && typeof field === "object" && "Valid" in field) {
      if (!field.Valid) return null;
      if ("Int64" in field) return field.Int64;
      if ("String" in field) return field.String;
      if ("Bool" in field) return field.Bool;
      if ("Float64" in field) return field.Float64;
      return null;
    }
    return field === undefined ? null : field;
  }

  function setStatus(message, isError) {
    if (!message) {
      els.apiStatus.hidden = true;
      return;
    }
    els.apiStatus.hidden = false;
    els.apiStatus.textContent = message;
    els.apiStatus.classList.toggle("error-text", !!isError);
  }

  // ---- Tabs ---------------------------------------------------------------

  els.navTabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      var view = tab.getAttribute("data-view");
      els.navTabs.forEach(function (t) {
        var active = t === tab;
        t.classList.toggle("is-active", active);
        t.setAttribute("aria-selected", String(active));
      });
      Object.keys(els.views).forEach(function (key) {
        els.views[key].hidden = key !== view;
        els.views[key].classList.toggle("is-active", key === view);
      });
    });
  });

  // ---- Dataset label formatting -------------------------------------------

  function formatDatasetLabel(routeSlug) {
    return routeSlug
      .replace(/[-_]/g, " ")
      .replace(/\b\w/g, function (c) {
        return c.toUpperCase();
      });
  }

  // ---- Generic dataset loading (shared by NUTS + LAU) --------------------

  // Tries the gap-free /{slug}/timeseries endpoint first (uniform schema,
  // interpolation metadata included); falls back to the raw dataset route
  // for the handful of datasets that don't support it (e.g.
  // beschaeftigungsquote-gemeinden).
  function loadDatasetSeries(routeSlug, nutsCode) {
    return apiGet("/" + routeSlug + "/timeseries", { geodata_id_nuts: nutsCode, limit: "null" })
      .then(function (body) {
        return { kind: "timeseries", rows: body.data || [] };
      })
      .catch(function () {
        return apiGet("/" + routeSlug, { geodata_id_nuts: nutsCode, limit: "null" }).then(function (body) {
          return { kind: "raw", rows: body.data || [] };
        });
      });
  }

  function renderDatasetContent(container, result) {
    container.innerHTML = "";
    if (!result.rows.length) {
      var hint = document.createElement("p");
      hint.className = "hint";
      hint.textContent = "Keine Werte für diesen Datensatz und diese Region vorhanden.";
      container.appendChild(hint);
      return;
    }
    if (result.kind === "timeseries") {
      renderTimeseries(container, result.rows);
    } else {
      renderGenericTable(container, result.rows);
    }
  }

  // ---- SVG line chart + table for /timeseries rows ------------------------

  function renderTimeseries(container, rows) {
    var points = rows.slice().sort(function (a, b) {
      return a.year - b.year;
    });

    var chartWrap = document.createElement("div");
    chartWrap.className = "chart-wrap";
    var svgNS = "http://www.w3.org/2000/svg";
    var svg = document.createElementNS(svgNS, "svg");
    svg.setAttribute("viewBox", "0 0 640 260");
    svg.setAttribute("preserveAspectRatio", "none");
    chartWrap.appendChild(svg);
    container.appendChild(chartWrap);

    var caption = document.createElement("p");
    caption.className = "chart-caption";
    caption.innerHTML =
      '<span><span class="marker marker-observed"></span>beobachtet</span>' +
      '<span><span class="marker marker-interpolated"></span>interpoliert</span>';
    container.appendChild(caption);

    drawLineChart(svg, points);

    renderTimeseriesTable(container, points);
  }

  function drawLineChart(svg, points) {
    var svgNS = "http://www.w3.org/2000/svg";
    var padding = { top: 16, right: 16, bottom: 28, left: 72 };
    var width = 640;
    var height = 260;
    var plotW = width - padding.left - padding.right;
    var plotH = height - padding.top - padding.bottom;

    var years = points.map(function (p) {
      return p.year;
    });
    var values = points.map(function (p) {
      return p.value;
    });
    var minYear = Math.min.apply(null, years);
    var maxYear = Math.max.apply(null, years);
    var minValue = Math.min.apply(null, values);
    var maxValue = Math.max.apply(null, values);
    if (minValue === maxValue) {
      minValue -= 1;
      maxValue += 1;
    }
    var valuePad = (maxValue - minValue) * 0.1;
    minValue -= valuePad;
    maxValue += valuePad;

    function xFor(year) {
      if (maxYear === minYear) return padding.left + plotW / 2;
      return padding.left + ((year - minYear) / (maxYear - minYear)) * plotW;
    }
    function yFor(value) {
      return padding.top + plotH - ((value - minValue) / (maxValue - minValue)) * plotH;
    }

    // Gridlines + y ticks
    var tickCount = 4;
    for (var i = 0; i <= tickCount; i++) {
      var value = minValue + ((maxValue - minValue) * i) / tickCount;
      var y = yFor(value);
      var line = document.createElementNS(svgNS, "line");
      line.setAttribute("x1", padding.left);
      line.setAttribute("x2", width - padding.right);
      line.setAttribute("y1", y);
      line.setAttribute("y2", y);
      line.setAttribute("stroke", "var(--gridline)");
      line.setAttribute("stroke-width", "1");
      svg.appendChild(line);

      var label = document.createElementNS(svgNS, "text");
      label.setAttribute("x", padding.left - 8);
      label.setAttribute("y", y + 3);
      label.setAttribute("text-anchor", "end");
      label.setAttribute("font-size", "10");
      label.setAttribute("fill", "var(--text-muted)");
      label.textContent = value.toLocaleString("de-DE", { maximumFractionDigits: 0 });
      svg.appendChild(label);
    }

    // Baseline
    var baseline = document.createElementNS(svgNS, "line");
    baseline.setAttribute("x1", padding.left);
    baseline.setAttribute("x2", width - padding.right);
    baseline.setAttribute("y1", height - padding.bottom);
    baseline.setAttribute("y2", height - padding.bottom);
    baseline.setAttribute("stroke", "var(--baseline)");
    baseline.setAttribute("stroke-width", "1");
    svg.appendChild(baseline);

    // X-axis year labels (first, middle, last)
    var labelIndexes = [0, Math.floor(points.length / 2), points.length - 1];
    labelIndexes.forEach(function (idx) {
      if (idx < 0 || idx >= points.length) return;
      var p = points[idx];
      var text = document.createElementNS(svgNS, "text");
      text.setAttribute("x", xFor(p.year));
      text.setAttribute("y", height - padding.bottom + 18);
      text.setAttribute("text-anchor", "middle");
      text.setAttribute("font-size", "10");
      text.setAttribute("fill", "var(--text-muted)");
      text.textContent = p.year;
      svg.appendChild(text);
    });

    // Line path
    var pathData = points
      .map(function (p, i) {
        return (i === 0 ? "M" : "L") + xFor(p.year) + " " + yFor(p.value);
      })
      .join(" ");
    var path = document.createElementNS(svgNS, "path");
    path.setAttribute("d", pathData);
    path.setAttribute("fill", "none");
    path.setAttribute("stroke", "var(--series-1)");
    path.setAttribute("stroke-width", "2");
    path.setAttribute("stroke-linecap", "round");
    path.setAttribute("stroke-linejoin", "round");
    svg.appendChild(path);

    // Markers + hover targets
    points.forEach(function (p) {
      var cx = xFor(p.year);
      var cy = yFor(p.value);
      var circle = document.createElementNS(svgNS, "circle");
      circle.setAttribute("cx", cx);
      circle.setAttribute("cy", cy);
      circle.setAttribute("r", 4);
      if (p.is_interpolated) {
        circle.setAttribute("fill", "var(--surface-1)");
        circle.setAttribute("stroke", "var(--series-1)");
        circle.setAttribute("stroke-width", "2");
      } else {
        circle.setAttribute("fill", "var(--series-1)");
      }
      svg.appendChild(circle);

      var hitTarget = document.createElementNS(svgNS, "circle");
      hitTarget.setAttribute("cx", cx);
      hitTarget.setAttribute("cy", cy);
      hitTarget.setAttribute("r", 10);
      hitTarget.setAttribute("fill", "transparent");
      hitTarget.style.cursor = "pointer";
      hitTarget.addEventListener("mouseenter", function (evt) {
        showTooltip(evt, p);
      });
      hitTarget.addEventListener("mousemove", function (evt) {
        showTooltip(evt, p);
      });
      hitTarget.addEventListener("mouseleave", hideTooltip);
      svg.appendChild(hitTarget);
    });
  }

  function showTooltip(evt, point) {
    var rows = [["Jahr", point.year], ["Wert", formatNumber(point.value)]];
    rows.push(["Status", point.is_interpolated ? "interpoliert" : "beobachtet"]);
    if (point.is_interpolated) {
      if (point.value_linear !== undefined && point.value_linear !== null) {
        rows.push(["linear", formatNumber(point.value_linear)]);
      }
      if (point.value_peer_trend !== undefined && point.value_peer_trend !== null) {
        rows.push(["Peer-Trend", formatNumber(point.value_peer_trend)]);
      }
      if (point.n_peer_regions !== undefined && point.n_peer_regions !== null) {
        rows.push(["Peer-Regionen", point.n_peer_regions]);
      }
    }
    els.tooltip.innerHTML =
      "<dl>" +
      rows
        .map(function (r) {
          return "<dt>" + r[0] + "</dt><dd>" + r[1] + "</dd>";
        })
        .join("") +
      "</dl>";
    els.tooltip.hidden = false;
    els.tooltip.style.left = evt.clientX + 12 + "px";
    els.tooltip.style.top = evt.clientY + 12 + "px";
  }

  function hideTooltip() {
    els.tooltip.hidden = true;
  }

  function renderTimeseriesTable(container, points) {
    var wrap = document.createElement("div");
    wrap.className = "data-table-wrap";
    var table = document.createElement("table");
    table.className = "data-table";
    table.innerHTML =
      "<thead><tr><th>Jahr</th><th>Wert</th><th>Status</th><th>Linear</th><th>Peer-Trend</th></tr></thead>";
    var tbody = document.createElement("tbody");
    points.forEach(function (p) {
      var tr = document.createElement("tr");
      if (p.is_interpolated) tr.className = "is-interpolated";
      tr.innerHTML =
        "<td>" +
        p.year +
        "</td><td>" +
        formatNumber(p.value) +
        "</td><td>" +
        (p.is_interpolated ? "interpoliert" : "beobachtet") +
        "</td><td>" +
        (p.value_linear !== undefined && p.value_linear !== null ? formatNumber(p.value_linear) : "–") +
        "</td><td>" +
        (p.value_peer_trend !== undefined && p.value_peer_trend !== null
          ? formatNumber(p.value_peer_trend)
          : "–") +
        "</td>";
      tbody.appendChild(tr);
    });
    table.appendChild(tbody);
    wrap.appendChild(table);
    container.appendChild(wrap);
  }

  // ---- Generic fallback table for raw dataset rows ------------------------

  function renderGenericTable(container, rows) {
    var skipKeys = ["mapper"];
    var columns = Object.keys(rows[0]).filter(function (key) {
      return skipKeys.indexOf(key) === -1;
    });

    var wrap = document.createElement("div");
    wrap.className = "data-table-wrap";
    var table = document.createElement("table");
    table.className = "data-table";
    var thead = document.createElement("thead");
    thead.innerHTML = "<tr>" + columns.map(function (c) { return "<th>" + c + "</th>"; }).join("") + "</tr>";
    table.appendChild(thead);

    var tbody = document.createElement("tbody");
    rows.forEach(function (row) {
      var tr = document.createElement("tr");
      tr.innerHTML = columns
        .map(function (c) {
          return "<td>" + formatCell(row[c], c) + "</td>";
        })
        .join("");
      tbody.appendChild(tr);
    });
    table.appendChild(tbody);
    wrap.appendChild(table);
    container.appendChild(wrap);
  }

  // Columns holding calendar years/IDs should not get thousands separators.
  var UNGROUPED_NUMBER_COLUMNS = ["year", "geodata_year"];

  function formatCell(value, column) {
    if (value === null || value === undefined) return "–";
    if (typeof value === "boolean") return value ? "Ja" : "Nein";
    if (typeof value === "number") {
      return UNGROUPED_NUMBER_COLUMNS.indexOf(column) !== -1 ? String(value) : formatNumber(value);
    }
    if (typeof value === "object") return JSON.stringify(value);
    return String(value);
  }

  function formatNumber(value) {
    if (typeof value !== "number") return String(value);
    return value.toLocaleString("de-DE", { maximumFractionDigits: 2 });
  }

  // ---- Dataset picker (shared) --------------------------------------------

  function renderDatasetPicker(container, datasets, nutsCode) {
    var picker = document.createElement("div");
    picker.className = "dataset-picker";
    var label = document.createElement("label");
    label.setAttribute("for", "dataset-select");
    label.textContent = "Datensatz";
    var select = document.createElement("select");
    select.id = "dataset-select";
    datasets.forEach(function (ds) {
      var option = document.createElement("option");
      option.value = ds.route_slug;
      option.textContent = formatDatasetLabel(ds.route_slug) + " (" + ds.available_years.length + " Jahre)";
      select.appendChild(option);
    });
    picker.appendChild(label);
    picker.appendChild(select);
    container.appendChild(picker);

    var content = document.createElement("div");
    container.appendChild(content);

    function loadSelected() {
      content.innerHTML = '<p class="hint">Lade Daten…</p>';
      loadDatasetSeries(select.value, nutsCode)
        .then(function (result) {
          renderDatasetContent(content, result);
        })
        .catch(function (err) {
          content.innerHTML = '<p class="error-text">Fehler beim Laden: ' + err.message + "</p>";
        });
    }

    select.addEventListener("change", loadSelected);
    loadSelected();
  }

  function renderNoDatasets(container) {
    var hint = document.createElement("p");
    hint.className = "hint";
    hint.textContent = "Für diese Region sind keine sozioökonomischen Datensätze verfügbar.";
    container.appendChild(hint);
  }

  // ---- NUTS-3 view ---------------------------------------------------------

  var allNutsRegions = [];
  var selectedNutsId = null;

  function loadNutsRegions() {
    apiGet("/nuts", { level: 3, limit: "null" })
      .then(function (body) {
        allNutsRegions = body.data || [];
        renderNutsList("");
      })
      .catch(function (err) {
        els.nutsList.innerHTML = '<p class="error-text">Regionen konnten nicht geladen werden: ' + err.message + "</p>";
      });
  }

  function renderNutsList(filterText) {
    var needle = filterText.trim().toLowerCase();
    var filtered = !needle
      ? allNutsRegions
      : allNutsRegions.filter(function (r) {
          return r.region_name.toLowerCase().indexOf(needle) !== -1 || r.nuts_id.toLowerCase().indexOf(needle) !== -1;
        });

    els.nutsList.innerHTML = "";
    if (!filtered.length) {
      els.nutsList.innerHTML = '<p class="hint">Keine Treffer.</p>';
      return;
    }

    var shown = filtered.slice(0, 300);
    shown.forEach(function (region) {
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "region-item" + (region.nuts_id === selectedNutsId ? " is-selected" : "");
      btn.innerHTML = region.region_name + '<span class="region-code">' + region.nuts_id + "</span>";
      btn.addEventListener("click", function () {
        selectNutsRegion(region);
      });
      els.nutsList.appendChild(btn);
    });
    if (filtered.length > shown.length) {
      var more = document.createElement("p");
      more.className = "hint";
      more.textContent = filtered.length - shown.length + " weitere Treffer – Suche eingrenzen.";
      els.nutsList.appendChild(more);
    }
  }

  function selectNutsRegion(region) {
    selectedNutsId = region.nuts_id;
    renderNutsList(els.nutsSearch.value);

    els.nutsDetail.innerHTML = "";
    var header = document.createElement("div");
    header.className = "detail-header";
    header.innerHTML =
      "<h2>" +
      region.region_name +
      '</h2><span class="region-code">' +
      region.nuts_id +
      (region.ars ? " · ARS " + region.ars : "") +
      "</span>";
    els.nutsDetail.appendChild(header);

    var pickerContainer = document.createElement("div");
    els.nutsDetail.appendChild(pickerContainer);
    pickerContainer.innerHTML = '<p class="hint">Lade verfügbare Datensätze…</p>';

    apiGet("/nuts/" + region.nuts_id + "/datasets")
      .then(function (body) {
        pickerContainer.innerHTML = "";
        var datasets = (body.data && body.data.datasets) || [];
        if (!datasets.length) {
          renderNoDatasets(pickerContainer);
          return;
        }
        renderDatasetPicker(pickerContainer, datasets, region.nuts_id);
      })
      .catch(function (err) {
        pickerContainer.innerHTML = '<p class="error-text">Fehler: ' + err.message + "</p>";
      });
  }

  els.nutsSearch.addEventListener("input", function () {
    renderNutsList(els.nutsSearch.value);
  });

  // ---- LAU view -------------------------------------------------------------

  var lauSearchTimer = null;

  els.lauSearch.addEventListener("input", function () {
    var query = els.lauSearch.value.trim();
    window.clearTimeout(lauSearchTimer);
    if (query.length < 2) {
      els.lauList.innerHTML = '<p class="hint">Gemeindenamen eingeben, um zu suchen.</p>';
      return;
    }
    lauSearchTimer = window.setTimeout(function () {
      searchLau(query);
    }, 350);
  });

  var lastLauResults = [];

  function searchLau(query) {
    els.lauList.innerHTML = '<p class="hint">Suche…</p>';
    apiGet("/lau", { geodata_name_contains: query, limit: 20 })
      .then(function (body) {
        lastLauResults = dedupeLauByGiscoId(body.data || []);
        renderLauList(lastLauResults);
      })
      .catch(function (err) {
        els.lauList.innerHTML = '<p class="error-text">Suche fehlgeschlagen: ' + err.message + "</p>";
      });
  }

  // /lau returns one row per (gisco_id, year); the search UI only needs one
  // entry per region, so keep the most recent year per gisco_id.
  function dedupeLauByGiscoId(rows) {
    var byId = new Map();
    rows.forEach(function (row) {
      var existing = byId.get(row.gisco_id);
      if (!existing || row.year > existing.year) {
        byId.set(row.gisco_id, row);
      }
    });
    return Array.from(byId.values());
  }

  var selectedGiscoId = null;

  function renderLauList(results) {
    els.lauList.innerHTML = "";
    if (!results.length) {
      els.lauList.innerHTML = '<p class="hint">Keine Treffer.</p>';
      return;
    }
    results.forEach(function (region) {
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "region-item" + (region.gisco_id === selectedGiscoId ? " is-selected" : "");
      btn.innerHTML = region.lau_name + '<span class="region-code">' + region.gisco_id + "</span>";
      btn.addEventListener("click", function () {
        selectLauRegion(region.gisco_id);
      });
      els.lauList.appendChild(btn);
    });
  }

  function selectLauRegion(giscoId) {
    selectedGiscoId = giscoId;
    renderLauList(lastLauResults);

    els.lauDetail.innerHTML = '<p class="hint">Lade Details…</p>';
    apiGet("/lau/" + giscoId)
      .then(function (body) {
        renderLauDetail(body.data);
      })
      .catch(function (err) {
        els.lauDetail.innerHTML = '<p class="error-text">Fehler: ' + err.message + "</p>";
      });
  }

  function renderLauDetail(region) {
    els.lauDetail.innerHTML = "";
    var header = document.createElement("div");
    header.className = "detail-header";
    header.innerHTML =
      "<h2>" + region.lau_name + '</h2><span class="region-code">' + region.gisco_id + "</span>";
    els.lauDetail.appendChild(header);

    var population = unwrapNull(region.population_count);
    var country = unwrapNull(region.country) || region.country_code;
    var nuts3 = unwrapNull(region.nuts_3_code);

    var meta = document.createElement("dl");
    meta.className = "meta-grid";
    var entries = [
      ["Land", country],
      ["Jahr", region.year],
      ["Bevölkerung", population !== null ? formatNumber(population) : "unbekannt"],
      ["NUTS-3", nuts3 || "keine Zuordnung"],
    ];
    entries.forEach(function (entry) {
      var dt = document.createElement("dt");
      dt.textContent = entry[0];
      var dd = document.createElement("dd");
      dd.textContent = entry[1] === null || entry[1] === undefined ? "–" : entry[1];
      meta.appendChild(dt);
      meta.appendChild(dd);
    });
    els.lauDetail.appendChild(meta);

    var pickerContainer = document.createElement("div");
    els.lauDetail.appendChild(pickerContainer);

    if (!nuts3) {
      pickerContainer.innerHTML =
        '<p class="hint">Keine NUTS-3-Zuordnung vorhanden – keine sozioökonomischen Datensätze verfügbar.</p>';
      return;
    }

    pickerContainer.innerHTML = '<p class="hint">Lade verfügbare Datensätze…</p>';
    apiGet("/lau/" + region.gisco_id + "/datasets")
      .then(function (body) {
        pickerContainer.innerHTML = "";
        var datasets = (body.data && body.data.datasets) || [];
        if (!datasets.length) {
          renderNoDatasets(pickerContainer);
          return;
        }
        renderDatasetPicker(pickerContainer, datasets, nuts3);
      })
      .catch(function (err) {
        pickerContainer.innerHTML = '<p class="error-text">Fehler: ' + err.message + "</p>";
      });
  }

  // ---- Startup --------------------------------------------------------------

  apiGet("/health")
    .then(function () {
      setStatus(null);
    })
    .catch(function (err) {
      setStatus("API unter " + API_BASE + " nicht erreichbar: " + err.message, true);
    });

  loadNutsRegions();
})();
