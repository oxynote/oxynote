export default defineI18nConfig(() => {
	return {
		fallbackWarn: false,
		missingWarn: false,
		datetimeFormats: {
			en: {
				"month-day-short": {
					month: "short",
					day: "numeric",
				},
				short: {
					year: "numeric",
					month: "short",
					day: "numeric",
				},
				long: {
					year: "numeric",
					month: "long",
					day: "numeric",
				},
				"short-with-time": {
					year: "numeric",
					month: "short",
					day: "numeric",
					hour: "numeric",
					minute: "numeric",
				},
				"hour-minute": {
					hour: "2-digit",
					minute: "2-digit",
					hourCycle: "h23",
				},
				"time-with-seconds": {
					hour: "2-digit",
					minute: "2-digit",
					second: "2-digit",
					hourCycle: "h23",
				},
				"short-with-full-time": {
					year: "numeric",
					month: "short",
					day: "numeric",
					hour: "numeric",
					minute: "numeric",
					second: "numeric",
				},
			},
		},
	}
})
