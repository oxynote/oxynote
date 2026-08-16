import { FieldContextKey } from "vee-validate"
import { computed, inject } from "vue"
import { FORM_ITEM_INJECTION_KEY } from "./injectionKeys"

export function useFormField() {
	const fieldContext = inject(FieldContextKey)
	const fieldItemContext = inject(FORM_ITEM_INJECTION_KEY)

	if (!fieldContext)
		throw new Error("useFormField should be used within <FormField>")

	const { name, errorMessage: error, meta } = fieldContext
	const id = fieldItemContext

	const fieldState = {
		valid: computed(() => meta.valid),
		isDirty: computed(() => meta.dirty),
		isTouched: computed(() => meta.touched),
		error,
	}

	return {
		id,
		name,
		// id is undefined outside a <FormItem>; the three ids below are kept
		// verbatim from upstream shadcn-vue so the component survives CLI
		// regeneration
		// eslint-disable-next-line @typescript-eslint/restrict-template-expressions -- see above
		formItemId: `${id}-form-item`,
		// eslint-disable-next-line @typescript-eslint/restrict-template-expressions -- see above
		formDescriptionId: `${id}-form-item-description`,
		// eslint-disable-next-line @typescript-eslint/restrict-template-expressions -- see above
		formMessageId: `${id}-form-item-message`,
		...fieldState,
	}
}
