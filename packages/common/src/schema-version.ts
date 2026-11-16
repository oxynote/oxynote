// the editor schema version a client needs to write. y-prosemirror deletes
// every node its schema does not know, and drops unknown attributes from
// every node it changes. So auth-realtime opens a client with an older
// version read-only. Raise it when a node type or an attribute is added or
// renamed.
export const EDITOR_SCHEMA_VERSION = 3
