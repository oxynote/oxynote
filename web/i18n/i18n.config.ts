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
				// a log line's time, the fraction written as milliseconds. A log
				// row swaps it for the exact digits.
				"log-time": {
					hour: "2-digit",
					minute: "2-digit",
					second: "2-digit",
				},
				"log-time-fraction": {
					hour: "2-digit",
					minute: "2-digit",
					second: "2-digit",
					fractionalSecondDigits: 3,
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
