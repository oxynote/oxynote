export function useAIChat() {
	const { t } = useI18n({ useScope: "global" })
	const config = useRuntimeConfig()
	const messages = ref<ChatMessage[]>([])
	const isStreaming = ref(false)
	const toolStatus = ref<string | null>(null)

	// pendingConfirm is the batch of writes awaiting the user's answer.
	// The server keeps its own copy, so a reconnect re-delivers it and
	// the question survives a page reload.
	const pendingConfirm = ref<ConfirmRequest | null>(null)

	// streamingText accumulates the current run of assistant text. It is
	// only committed to the chat once the server says the run was a
	// final reply rather than narration before a tool call.
	const streamingText = ref("")

	// activeDocumentId is echoed to the server on connect and whenever
	// it changes, so the model can resolve "this document".
	const activeDocumentId = ref<string | null>(null)
	const activeBranchId = ref<string | null>(null)

	const wsUrl = computed(() => {
		const base = config.public.coreAPIBaseWsURL
		return `${base}/api/ai/chat`
	})

	const { status, send, open, close } = useWebSocket(wsUrl, {
		immediate: false,
		autoClose: false,
		onDisconnected: () => {
			isStreaming.value = false
			toolStatus.value = null
			streamingText.value = ""
		},
		onConnected: () => {
			sendActiveDocument()
		},
		autoReconnect: {
			delay: 2500,
		},
		onMessage: (_: WebSocket, event: MessageEvent) => {
			handleMessage(event)
		},
	})

	const isConnected = computed(() => status.value === "OPEN")

	function connect() {
		open()
	}

	function disconnect() {
		close()
		messages.value = []
		isStreaming.value = false
		toolStatus.value = null
		streamingText.value = ""
		pendingConfirm.value = null
	}

	function sendMessage(content: string) {
		if (!isConnected.value) {
			return
		}

		messages.value.push({ role: ChatMessageRole.User, text: content })
		isStreaming.value = true
		streamingText.value = ""

		send(JSON.stringify({ type: ClientMessageType.Message, content }))
	}

	// setActiveDocument records which document, and which branch of it,
	// the user is looking at. Called on navigation; the value is also
	// replayed on reconnect.
	function setActiveDocument(
		documentId: string | null,
		branchId: string | null = null,
	) {
		activeDocumentId.value = documentId
		activeBranchId.value = branchId
		sendActiveDocument()
	}

	function sendActiveDocument() {
		if (!isConnected.value) {
			return
		}

		send(
			JSON.stringify({
				type: ClientMessageType.SetActiveDocument,
				documentId: activeDocumentId.value ?? "",
				branchId: activeBranchId.value ?? "",
			}),
		)
	}

	// answerConfirm approves or declines the outstanding batch of writes.
	// "all" approves the rest of this turn as well, which the server
	// still overrides for deletes.
	function answerConfirm(approved: boolean, all = false) {
		const pending = pendingConfirm.value
		if (!pending || !isConnected.value) {
			return
		}

		pendingConfirm.value = null
		isStreaming.value = true

		send(
			JSON.stringify({
				type: ClientMessageType.ConfirmResponse,
				turnId: pending.turnId,
				approved,
				all,
			}),
		)
	}

	function pushError(text: string) {
		messages.value.push({ role: ChatMessageRole.Error, text })
	}

	// commitStreamedText moves the accumulated run into the chat. A run
	// that merely narrated an upcoming tool call is dropped instead.
	function commitStreamedText(kind: TextEndKind) {
		const text = streamingText.value
		streamingText.value = ""

		if (kind !== TextEndKind.Message || !text) {
			return
		}

		messages.value.push({ role: ChatMessageRole.Assistant, text })
	}

	function handleMessage(event: MessageEvent) {
		let msg: ServerMessage

		try {
			msg = JSON.parse(event.data as string) as ServerMessage
		} catch {
			return
		}

		switch (msg.type) {
			case ServerMessageType.History: {
				if (msg.messages) {
					messages.value = msg.messages.map((entry) => ({
						role: entry.role,
						text: entry.content,
					}))
				}

				break
			}
			case ServerMessageType.TextDelta: {
				streamingText.value += msg.content ?? ""
				toolStatus.value = null

				break
			}
			case ServerMessageType.TextEnd: {
				commitStreamedText(msg.kind ?? TextEndKind.Message)

				break
			}
			case ServerMessageType.ToolStatus: {
				toolStatus.value = msg.label ?? t("editor.ai-chat.tool-status.working")

				break
			}
			case ServerMessageType.ConfirmRequest: {
				if (!msg.turnId) {
					break
				}

				pendingConfirm.value = {
					turnId: msg.turnId,
					actions: msg.actions ?? [],
				}

				// the turn is parked until the user answers, so stop
				// showing it as working.
				isStreaming.value = false
				toolStatus.value = null

				break
			}
			case ServerMessageType.Done: {
				isStreaming.value = false
				toolStatus.value = null
				streamingText.value = ""

				break
			}
			case ServerMessageType.Error: {
				pushError(msg.message ?? t("editor.ai-chat.generic-error"))

				isStreaming.value = false
				toolStatus.value = null
				streamingText.value = ""

				break
			}
		}
	}

	function resetChat() {
		messages.value = []
		isStreaming.value = false
		toolStatus.value = null
		streamingText.value = ""
		pendingConfirm.value = null

		if (isConnected.value) {
			send(JSON.stringify({ type: ClientMessageType.Reset }))
		}
	}

	return {
		messages,
		isConnected,
		isStreaming,
		toolStatus,
		streamingText,
		pendingConfirm,
		connect,
		disconnect,
		sendMessage,
		setActiveDocument,
		answerConfirm,
		resetChat,
	}
}
