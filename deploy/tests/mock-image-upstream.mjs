import http from "node:http";

const host = process.env.MOCK_IMAGE_HOST || "0.0.0.0";
const port = Number(process.env.MOCK_IMAGE_PORT || 19090);
const validPng = "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAcSURBVDhPY1BIWPCfEjxqwKgBIDxqwDAwYMF/AM4THx/xRTzQAAAAAElFTkSuQmCC";

function sendJson(response, status, body) {
    response.writeHead(status, { "Content-Type": "application/json" });
    response.end(JSON.stringify(body));
}

function readBody(request) {
    return new Promise((resolve, reject) => {
        const chunks = [];
        request.on("data", (chunk) => chunks.push(chunk));
        request.on("end", () => resolve(Buffer.concat(chunks)));
        request.on("error", reject);
    });
}

function requestedDelay(raw) {
    const match = raw.toString("utf8").match(/__mock_delay_ms_(\d+)__/);
    return match ? Math.min(Number(match[1]), 30_000) : 0;
}

const server = http.createServer(async (request, response) => {
    if (request.method === "GET" && request.url === "/health") return sendJson(response, 200, { ok: true });
    if (request.method === "GET" && request.url === "/v1/models") {
        return sendJson(response, 200, { object: "list", data: [{ id: "gpt-image-2", object: "model" }] });
    }

    const raw = await readBody(request);
    if (raw.includes(Buffer.from("__mock_fail_500__"))) return sendJson(response, 500, { error: { message: "mock image upstream failure" } });
    const delay = requestedDelay(raw);
    if (delay) await new Promise((resolve) => setTimeout(resolve, delay));

    if (request.method === "POST" && (request.url === "/v1/images/generations" || request.url === "/v1/images/edits")) {
        return sendJson(response, 200, {
            created: Math.floor(Date.now() / 1000),
            data: [{ b64_json: validPng }],
            usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 },
        });
    }

    return sendJson(response, 404, { error: { message: `mock route not found: ${request.method} ${request.url}` } });
});

server.listen(port, host, () => process.stdout.write(`mock image upstream listening on http://${host}:${port}\n`));
