import { Hono } from "https://esm.sh/hono@3.11.7";
const app = new Hono();

// Unwrap Hono errors to see original error details
app.onError((err) => {
  throw err;
});
app.route("/echo").all(async (c) => {
  const res = await c.req.text();
  console.log(res);
  return c.text(res);
});

// 404 handler
app.notFound((c) => {
  return c.json({ error: "Not found" }, 404);
});

export default { fetch: app.fetch };
