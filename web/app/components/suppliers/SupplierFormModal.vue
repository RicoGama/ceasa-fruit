<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'
import type { Supplier } from '~/types'

// supplier = null -> criar; supplier preenchido -> editar.
const props = defineProps<{ supplier: Supplier | null }>()
const emit = defineEmits<{ saved: [supplier: Supplier] }>()

// defineModel cria o v-model:open do componente (abrir/fechar o modal).
const open = defineModel<boolean>('open', { default: false })

const api = useApi()
const toast = useToast()
const form = useTemplateRef('form')
const saving = ref(false)

const states = ['AC', 'AL', 'AM', 'AP', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA', 'MG', 'MS', 'MT', 'PA', 'PB',
  'PE', 'PI', 'PR', 'RJ', 'RN', 'RO', 'RR', 'RS', 'SC', 'SE', 'SP', 'TO']

// Mesmas regras da API, com mensagens em português. A API valida de novo:
// isto é só para o usuário ver o erro antes de enviar.
const schema = z.object({
  name: z.string().trim().min(2, 'Informe pelo menos 2 caracteres').max(120, 'Máximo de 120 caracteres'),
  document: z.string().refine(isValidDocument, 'CPF ou CNPJ inválido'),
  phone: z.string().max(20, 'Máximo de 20 caracteres').optional(),
  email: z.union([z.literal(''), z.email('E-mail inválido')]).optional(),
  city: z.string().max(80, 'Máximo de 80 caracteres').optional(),
  state: z.string().optional(),
  notes: z.string().max(1000, 'Máximo de 1000 caracteres').optional(),
  active: z.boolean(),
})
type Schema = z.output<typeof schema>

function emptyState(): Schema {
  return { name: '', document: '', phone: '', email: '', city: '', state: undefined, notes: '', active: true }
}

const state = reactive<Schema>(emptyState())

// Toda vez que o modal abre, preenche o formulário com o fornecedor (ou limpa).
watch(open, (isOpen) => {
  if (!isOpen) return
  const s = props.supplier
  Object.assign(state, s
    ? {
        name: s.name,
        document: formatDocument(s.document),
        phone: s.phone ?? '',
        email: s.email ?? '',
        city: s.city ?? '',
        state: s.state ?? undefined,
        notes: s.notes ?? '',
        active: s.active,
      }
    : emptyState())
  form.value?.clear()
})

// "" vira undefined: a API trata campo ausente como NULL no banco.
const orUndefined = (v?: string) => (v?.trim() ? v.trim() : undefined)

async function onSubmit(event: FormSubmitEvent<Schema>) {
  const v = event.data
  const fields = {
    name: v.name,
    document: v.document,
    phone: orUndefined(v.phone),
    email: orUndefined(v.email),
    city: orUndefined(v.city),
    state: orUndefined(v.state),
    notes: orUndefined(v.notes),
  }

  saving.value = true
  const result = props.supplier
    ? await api.PUT('/suppliers/{id}', {
        params: { path: { id: props.supplier.id } },
        body: { ...fields, active: v.active },
      })
    : await api.POST('/suppliers', { body: fields })
  saving.value = false

  if (result.error) {
    if (result.response.status === 409) {
      form.value?.setErrors([{ name: 'document', message: apiErrorMessage(result.error) }])
    }
    else {
      form.value?.setErrors(apiFieldErrors(result.error))
      toast.add({ title: apiErrorMessage(result.error), color: 'error' })
    }
    return
  }

  toast.add({ title: props.supplier ? 'Fornecedor atualizado' : 'Fornecedor cadastrado', color: 'success' })
  emit('saved', result.data)
  open.value = false
}
</script>

<template>
  <UModal
    v-model:open="open"
    :title="supplier ? 'Editar fornecedor' : 'Novo fornecedor'"
    :ui="{ footer: 'justify-end' }"
  >
    <template #body>
      <UForm id="supplier-form" ref="form" :schema="schema" :state="state" class="space-y-4" @submit="onSubmit">
        <UFormField label="Nome" name="name" required>
          <UInput v-model="state.name" placeholder="Sítio Boa Colheita" class="w-full" autofocus />
        </UFormField>

        <UFormField label="CPF ou CNPJ" name="document" required help="Produtor rural pode usar CPF">
          <UInput
            v-model="state.document"
            placeholder="00.000.000/0000-00"
            class="w-full"
            @blur="state.document = formatDocument(state.document)"
          />
        </UFormField>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <UFormField label="Telefone" name="phone">
            <UInput v-model="state.phone" type="tel" placeholder="(11) 99999-0000" class="w-full" />
          </UFormField>
          <UFormField label="E-mail" name="email">
            <UInput v-model="state.email" type="email" class="w-full" />
          </UFormField>
        </div>

        <div class="grid grid-cols-[1fr_7rem] gap-4">
          <UFormField label="Cidade" name="city">
            <UInput v-model="state.city" class="w-full" />
          </UFormField>
          <UFormField label="UF" name="state">
            <USelect v-model="state.state" :items="states" placeholder="UF" class="w-full" />
          </UFormField>
        </div>

        <UFormField label="Observações" name="notes">
          <UTextarea v-model="state.notes" :rows="3" class="w-full" />
        </UFormField>

        <UFormField v-if="supplier" name="active">
          <USwitch v-model="state.active" label="Fornecedor ativo" />
        </UFormField>
      </UForm>
    </template>

    <template #footer>
      <UButton label="Cancelar" color="neutral" variant="ghost" @click="open = false" />
      <UButton type="submit" form="supplier-form" label="Salvar" :loading="saving" />
    </template>
  </UModal>
</template>
