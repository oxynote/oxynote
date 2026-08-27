// the bundle entry: sentry first, then the service, folded into one file
// by build:bundle — as a separate --import preload the bundled app would
// carry a second sentry copy whose init it never sees. Excluded from the
// tsc build (tsconfig.build.json), which would otherwise emit a stray
// compiled copy into dist.
import "./sentry.js"
import "./index.js"
