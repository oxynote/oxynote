const TOOL_STATUS_I18N_KEYS: Record<AIToolName, string> = {
	[AIToolName.ReadDocument]: "editor.ai-chat.tool-status.read-document",
	[AIToolName.InsertBlocks]: "editor.ai-chat.tool-status.insert-blocks",
	[AIToolName.ReplaceBlockContent]:
		"editor.ai-chat.tool-status.replace-block-content",
	[AIToolName.ReplaceBlockAttributes]:
		"editor.ai-chat.tool-status.replace-block-attributes",
	[AIToolName.ReadAvailableIcons]:
		"editor.ai-chat.tool-status.read-available-icons",
	[AIToolName.UpdateDocumentName]:
		"editor.ai-chat.tool-status.update-document-name",
	[AIToolName.UpdateDocumentIcon]:
		"editor.ai-chat.tool-status.update-document-icon",
	[AIToolName.DeleteBlocks]: "editor.ai-chat.tool-status.delete-blocks",
}

export function useAIChat(opts: {
	toolExecutor: (tool: AIToolName, args: ExecuteToolArgs) => Promise<object>
}) {
	const { t } = useI18n({ useScope: "global" })
	const config = useRuntimeConfig()
	const messages = ref<ChatMessage[]>([])
	const isStreaming = ref(false)
	const toolStatus = ref<string | null>(null)

	const wsUrl = computed(() => {
		const base = config.public.coreAPIBaseWsURL as string
		return `${base}/api/ai/chat`
	})

	const { status, send, open, close } = useWebSocket(wsUrl, {
		immediate: false,
		autoClose: false,
		onDisconnected: () => {
			isStreaming.value = false
			toolStatus.value = null
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
	}

	function sendMessage(content: string) {
		if (!isConnected.value) {
			return
		}

		messages.value.push({ role: ChatMessageRole.User, text: content })
		isStreaming.value = true

		send(JSON.stringify({ type: ClientMessageType.Message, content }))
	}

	async function handleMessage(event: MessageEvent) {
		let msg: ServerMessage

		try {
			msg = JSON.parse(event.data as string)
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
				const last = messages.value[messages.value.length - 1]

				if (last && last.role === ChatMessageRole.Assistant) {
					last.text += msg.content ?? ""
				} else {
					messages.value.push({
						role: ChatMessageRole.Assistant,
						text: msg.content ?? "",
					})
				}

				break
			}
			case ServerMessageType.ToolCall: {
				if (!msg.id || !msg.tool) {
					break
				}

				toolStatus.value = t(
					TOOL_STATUS_I18N_KEYS[msg.tool] ??
						"editor.ai-chat.tool-status.working",
				)

				try {
					const result = await opts.toolExecutor(
						msg.tool,
						msg.args as ExecuteToolArgs,
					)

					if (isConnected.value) {
						send(
							JSON.stringify({
								type: ClientMessageType.ToolResult,
								id: msg.id,
								result,
							}),
						)
					}
				} catch (err) {
					if (isConnected.value) {
						send(
							JSON.stringify({
								type: ClientMessageType.ToolResult,
								id: msg.id,
								result: {
									error:
										err instanceof Error
											? err.message
											: "tool execution failed",
								},
							}),
						)
					}
				} finally {
					toolStatus.value = null
				}

				break
			}
			case ServerMessageType.Done: {
				isStreaming.value = false
				toolStatus.value = null

				break
			}
			case ServerMessageType.Error: {
				messages.value.push({
					role: ChatMessageRole.Error,
					text: msg.message ?? t("editor.ai-chat.generic-error"),
				})

				isStreaming.value = false
				toolStatus.value = null

				break
			}
		}
	}

	function resetChat() {
		messages.value = []
		isStreaming.value = false
		toolStatus.value = null

		if (isConnected.value) {
			send(JSON.stringify({ type: ClientMessageType.Reset }))
		}
	}

	return {
		messages,
		isConnected,
		isStreaming,
		toolStatus,
		connect,
		disconnect,
		sendMessage,
		resetChat,
	}
}
