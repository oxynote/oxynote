export function useLastValidRef<T>(
	source: MaybeRefOrGetter<T | null | undefined>,
) {
	const last = ref<T | undefined>(undefined) // undefined is possible only until source is set

	watchImmediate(
		() => toValue(source),
		(v) => {
			if (v !== null && v !== undefined) {
				last.value = v
			}
		},
	)

	return readonly(last)
}
