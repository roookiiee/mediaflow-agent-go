const form = document.querySelector("#briefForm");
const timeline = document.querySelector("#timeline");
const artifacts = document.querySelector("#artifacts");
const jobs = document.querySelector("#jobs");
const provider = document.querySelector("#provider");
const quality = document.querySelector("#quality");
const cost = document.querySelector("#cost");
const button = form.querySelector("button");

async function loadHealth() {
  const response = await fetch("/api/health");
  const data = await response.json();
  provider.textContent = data.provider || "demo";
}

async function loadJobs() {
  const response = await fetch("/api/jobs");
  const data = await response.json();
  jobs.innerHTML = "";
  if (!Array.isArray(data) || data.length === 0) {
    jobs.innerHTML = '<div class="job"><strong>暂无历史任务</strong><span>运行一次 Agent 后会保存记录</span></div>';
    return;
  }
  for (const job of data.slice(0, 5)) {
    const item = document.createElement("div");
    item.className = "job";
    item.innerHTML = `<strong>${escapeHTML(job.channel || "Media")} · ${escapeHTML(job.target_locale || "")}</strong><span>${escapeHTML(job.summary || job.id)}</span>`;
    jobs.appendChild(item);
  }
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  timeline.innerHTML = "";
  artifacts.className = "artifacts empty";
  artifacts.textContent = "Agent 正在生成产物";
  quality.textContent = "--";
  cost.textContent = "--";
  button.disabled = true;

  const payload = {
    brief: form.brief.value,
    source_script: form.sourceScript.value,
    target_locale: form.targetLocale.value,
    channel: form.channel.value,
  };

  try {
    const response = await fetch("/api/agent/run", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });

    if (!response.ok || !response.body) {
      throw new Error(`request failed: ${response.status}`);
    }

    await readSSE(response.body.getReader(), (name, data) => {
      if (name === "done") {
        renderResult(data);
        loadJobs();
        return;
      }
      addEvent(name, data);
    });
  } catch (error) {
    addEvent("error", { message: error.message || String(error) });
    artifacts.className = "artifacts empty";
    artifacts.textContent = "运行失败";
  } finally {
    button.disabled = false;
  }
});

async function readSSE(reader, onEvent) {
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split("\n\n");
    buffer = frames.pop() || "";
    for (const frame of frames) {
      const parsed = parseFrame(frame);
      if (parsed) {
        onEvent(parsed.event, parsed.data);
      }
    }
  }
}

function parseFrame(frame) {
  const lines = frame.split("\n");
  let event = "message";
  let data = "";
  for (const line of lines) {
    if (line.startsWith("event:")) {
      event = line.slice(6).trim();
    }
    if (line.startsWith("data:")) {
      data += line.slice(5).trim();
    }
  }
  if (!data) {
    return null;
  }
  try {
    return { event, data: JSON.parse(data) };
  } catch {
    return { event, data: { message: data } };
  }
}

function addEvent(name, data) {
  const item = document.createElement("div");
  item.className = `event ${escapeHTML(name)}`;
  const title = data.tool || name;
  const duration = data.duration_ms ? `${data.duration_ms}ms` : "";
  item.innerHTML = `
    <div class="event-title">
      <span>${escapeHTML(title)}</span>
      <span>${escapeHTML(duration)}</span>
    </div>
    <div class="event-message">${escapeHTML(data.message || data.error || "")}</div>
  `;
  timeline.prepend(item);
}

function renderResult(result) {
  const metrics = result.metrics || {};
  quality.textContent = metrics.quality_score ? `${metrics.quality_score}/100` : "--";
  cost.textContent = metrics.estimated_cost_usd ? `$${metrics.estimated_cost_usd}` : "--";

  artifacts.className = "artifacts";
  artifacts.innerHTML = "";
  for (const artifact of result.artifacts || []) {
    const item = document.createElement("article");
    item.className = "artifact";
    const content =
      typeof artifact.content === "string"
        ? artifact.content
        : JSON.stringify(artifact.content, null, 2);
    item.innerHTML = `
      <header>
        <span>${escapeHTML(artifact.name)}</span>
        <span>${escapeHTML(artifact.kind)}</span>
      </header>
      <pre>${escapeHTML(content)}</pre>
    `;
    artifacts.appendChild(item);
  }
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

loadHealth().catch(() => {
  provider.textContent = "unknown";
});
loadJobs().catch(() => {
  jobs.innerHTML = '<div class="job"><strong>无法读取历史任务</strong><span>请确认后端服务已启动</span></div>';
});
