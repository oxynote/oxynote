interface APIEvent {
	topic: string
	payload: object
	success: boolean
	id: number
}

export function extractFromTopic(
	topic: string,
): [string, string, string[], string] {
	let strs = topic.split("~")
	if (strs.length != 2) {
		return ["", "", [], ""]
	}

	const descriptor = strs[0]!
	const cleanTopic = strs[1]!

	strs = strs[1]!.split("@")
	if (strs.length != 2) {
		return ["", "", [], ""]
	}

	const operation = strs[0]!

	return [descriptor, operation, strs.slice(1), cleanTopic]
}

interface TopicState {
	confirmed: boolean
	subIndex: number
	subs: Map<number, (payload: object) => void>
}

export default class WsState {
	private wsVal: WebSocket | null = null
	private readonly topics = new Map<string, TopicState>()
	private readonly pendingSubs = new Map<number, string>() // msgIndex:topic
	private msgIndex = 0
	private autoSubTimeout: number

	constructor(autoSubTimeout = 10000) {
		this.autoSubTimeout = autoSubTimeout
		this.runAutoSub()
	}

	set ws(ws: WebSocket | null) {
		this.wsVal = ws

		if (!ws) {
			this.topics.forEach((state: TopicState) => {
				state.confirmed = false
			})
		}
	}

	processMessage(msg: MessageEvent) {
		const ev = JSON.parse(msg.data, jsonReviver) as APIEvent

		// confirm a subscription
		if (ev.success) {
			const topic = this.pendingSubs.get(ev.id)
			if (!topic) {
				return
			}

			this.pendingSubs.delete(ev.id)

			const state = this.topics.get(topic)
			if (!state) {
				// the topic might not exist anymore
				// (early unsub)
				return
			}

			state.confirmed = true

			return
		}

		const topic = extractFromTopic(ev.topic)
		if (topic[0] !== "pub") {
			// accept only publish-related payloads
			return
		}

		const state = this.topics.get(topic[3])
		if (!state?.confirmed) {
			return
		}

		state.subs.forEach((fn: (payload: object) => void) => {
			fn(ev.payload)
		})
	}

	// subscribe subscribes to the provided topic and returns an
	// unsubscription function.
	subscribe(topic: string, fn: (payload: object) => void): () => void {
		let state = this.topics.get(topic)
		if (!state) {
			state = {
				confirmed: false,
				subIndex: 0,
				subs: new Map<number, (payload: object) => void>(),
			}
		} else {
			state.subIndex++
		}

		state.subs.set(state.subIndex, fn)
		this.topics.set(topic, state)
		this.subscribeUnconfirmed()

		// don't allow multiple unsubscriptions
		let unsubbed = false
		const delIndex = state.subIndex

		return () => {
			if (!this.topics.has(topic) || unsubbed) {
				return
			}

			const delState = this.topics.get(topic)!
			delState.subs.delete(delIndex)

			if (delState.subs.size === 0) {
				this.topics.delete(topic)
				this.send(false, topic)
			}

			unsubbed = true
		}
	}

	resubscribeAll() {
		this.topics.forEach((state: TopicState) => {
			state.confirmed = false
		})
		this.subscribeUnconfirmed()
	}

	private subscribeUnconfirmed() {
		this.topics.forEach((state: TopicState, topic: string) => {
			if (state.confirmed) {
				return
			}

			this.send(true, topic)
		})
	}

	private send(sub: boolean, topic: string) {
		if (!this.wsVal) {
			return
		}

		this.msgIndex++
		let descriptor = "unsub"

		if (sub) {
			descriptor = "sub"
			this.pendingSubs.set(this.msgIndex, topic)
		}

		const msg = {
			topic: `${descriptor}~${topic}`,
			id: this.msgIndex,
		}

		this.wsVal.send(jsonStableStringify(msg))
	}

	private runAutoSub() {
		setInterval(() => {
			this.subscribeUnconfirmed()
		}, this.autoSubTimeout)
	}
}
