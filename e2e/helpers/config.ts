// the ports the e2e stack publishes; they differ from the dev stack's so
// both can run at once. Changing one means changing docker-compose.yaml.
export const BASE_URL = "http://localhost:18080"
export const MAILPIT_URL = "http://localhost:18025"
export const REALTIME_WS_URL = `ws://localhost:18080/auth-realtime/hocuspocus`
