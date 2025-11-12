export default {
  fetch: async (req: Request): Promise<Response> => {
    if (req.headers.get("upgrade") === "websocket") {
      // For testing: to make sure that it doesn't matter exactly when we do the upgrade
      await new Promise((resolve) => setTimeout(resolve, Math.random()));

      const { socket, response } = Deno.upgradeWebSocket(req);

      socket.addEventListener("open", () => {
        console.log("WebSocket connection opened");
      });

      socket.addEventListener("message", (event: MessageEvent) => {
        console.log("Received message:", event.data);
        socket.send(event.data);
      });

      socket.addEventListener("error", (event: Event) => {
        console.error("WebSocket error:", event);
      });

      return response;
    }

    return new Response("Not a websocket request", { status: 400 });
  },
} satisfies Deno.ServeDefaultExport;
