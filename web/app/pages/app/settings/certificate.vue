<script setup lang="ts">
import type { CertificateInfo } from '~/types'

definePageMeta({ layout: 'dashboard', permission: 'certificate.manage' })
useHead({ title: 'Certificado digital · CEASA Fruit' })

// Certificado digital A1 da empresa: assina as notas fiscais e a comunicação
// com a SEFAZ. O arquivo e a senha ficam guardados criptografados no servidor
// do box (api/internal/certificate) e nunca voltam para a tela.

const api = useApi()
const toast = useToast()
const confirm = useConfirm()

const { data, error, refresh } = await useAsyncData('certificate', async () => {
  const { data, error } = await api.GET('/company/certificate')
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
})
useLiveRefresh('digital_certificates', refresh)

const current = computed(() => (data.value?.current ?? null) as CertificateInfo | null)
const history = computed(() => data.value?.history ?? [])
const keyConfigured = computed(() => data.value?.keyConfigured ?? true)

const statusUi = {
  valid: { color: 'success', label: 'Válido', icon: 'i-lucide-shield-check' },
  expiring: { color: 'warning', label: 'Vence em breve', icon: 'i-lucide-clock-alert' },
  expired: { color: 'error', label: 'Vencido', icon: 'i-lucide-shield-x' },
} as const

function validityText(c: CertificateInfo) {
  if (c.status === 'expired') return `Venceu em ${formatDate(c.notAfter)}`
  if (c.daysLeft <= 0) return `Vence hoje (${formatDate(c.notAfter)})`
  return `Até ${formatDate(c.notAfter)}, faltam ${daysText(c.daysLeft)}`
}

// --- Enviar / trocar ------------------------------------------------------------
const replacing = ref(false)
const showForm = computed(() => !current.value || replacing.value)
const file = ref<File | null>(null)
const password = ref('')
const passwordVisible = ref(false)
const sending = ref(false)
const errorMessage = ref('')

function resetForm() {
  file.value = null
  password.value = ''
  passwordVisible.value = false
  errorMessage.value = ''
}

function cancelReplace() {
  resetForm()
  replacing.value = false
}

async function send() {
  errorMessage.value = ''
  if (!file.value) {
    errorMessage.value = 'Escolha o arquivo do certificado (.pfx ou .p12).'
    return
  }
  if (!password.value) {
    errorMessage.value = 'Informe a senha do certificado.'
    return
  }
  const wasReplacing = !!current.value
  const chosen = file.value
  const secret = password.value
  sending.value = true
  const { error } = await api.POST('/company/certificate', {
    // O arquivo vai como formulário (multipart), não como JSON.
    body: { file: '', password: secret },
    bodySerializer() {
      const form = new FormData()
      form.append('file', chosen, chosen.name)
      form.append('password', secret)
      return form
    },
  })
  sending.value = false
  if (error) {
    errorMessage.value = apiErrorMessage(error)
    return
  }
  toast.add({ title: wasReplacing ? 'Certificado trocado' : 'Certificado enviado', color: 'success' })
  cancelReplace()
  await refresh()
}

// --- Remover ----------------------------------------------------------------------
const removing = ref(false)
async function remove() {
  const ok = await confirm({
    title: 'Remover o certificado?',
    description: 'Sem certificado, o sistema não emite notas fiscais até um novo ser enviado. Use só se o certificado foi perdido, vazou ou foi revogado.',
    confirmLabel: 'Remover',
    color: 'error',
    icon: 'i-lucide-trash-2',
  })
  if (!ok) return
  removing.value = true
  const { error } = await api.DELETE('/company/certificate')
  removing.value = false
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Certificado removido', color: 'success' })
  await refresh()
}
</script>

<template>
  <UDashboardPanel id="certificate">
    <template #header>
      <AppNavbar>
        <template #right>
          <AuditHistoryButton v-if="current" entity-type="certificate" :entity-id="current.id" title="Certificado digital" />
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <div class="mx-auto w-full max-w-4xl space-y-6">
        <UAlert v-if="error" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="error.message" />

        <UAlert
          v-if="!keyConfigured"
          color="warning"
          variant="subtle"
          icon="i-lucide-key-round"
          title="O servidor ainda não está preparado para guardar o certificado"
          description="Falta a chave de criptografia (SECRETS_KEY) na configuração do servidor. Rode a instalação de novo ou fale com o suporte."
        />

        <!-- Certificado em uso -->
        <UPageCard title="Certificado em uso" description="Assina as notas fiscais e a comunicação com a SEFAZ.">
          <div v-if="current" class="space-y-5">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="text-lg font-bold text-highlighted break-words">
                  {{ current.holderName }}
                </p>
                <p class="text-sm text-muted">
                  CNPJ {{ formatDocument(current.document ?? '') }}
                </p>
              </div>
              <UBadge
                :color="statusUi[current.status].color"
                :icon="statusUi[current.status].icon"
                :label="current.status === 'expiring' ? `Vence em ${daysText(current.daysLeft)}` : statusUi[current.status].label"
                variant="subtle"
                size="lg"
              />
            </div>

            <dl class="grid gap-x-6 gap-y-4 text-sm sm:grid-cols-2">
              <div>
                <dt class="text-muted">
                  Validade
                </dt>
                <dd class="font-medium text-highlighted">
                  {{ validityText(current) }}
                </dd>
              </div>
              <div>
                <dt class="text-muted">
                  Autoridade certificadora
                </dt>
                <dd class="font-medium text-highlighted break-words">
                  {{ current.issuer }}
                </dd>
              </div>
              <div>
                <dt class="text-muted">
                  Enviado
                </dt>
                <dd class="font-medium text-highlighted">
                  {{ formatDateTime(current.uploadedAt) }}<template v-if="current.uploadedBy">
                    por {{ current.uploadedBy.name }}
                  </template>
                </dd>
              </div>
              <div>
                <dt class="text-muted">
                  Número de série
                </dt>
                <dd class="font-mono text-xs text-highlighted break-all">
                  {{ current.serialNumber }}
                </dd>
              </div>
            </dl>

            <UAlert
              v-if="current.status !== 'valid'"
              :color="current.status === 'expired' ? 'error' : 'warning'"
              variant="subtle"
              icon="i-lucide-calendar-clock"
              :title="current.status === 'expired' ? 'Este certificado venceu: as notas fiscais não podem ser emitidas.' : 'Providencie a renovação com a certificadora e envie o novo arquivo aqui.'"
            />

            <div v-if="!replacing" class="flex flex-wrap gap-2">
              <UButton label="Trocar certificado" icon="i-lucide-refresh-cw" :disabled="!keyConfigured" @click="replacing = true" />
              <UButton label="Remover" icon="i-lucide-trash-2" color="error" variant="ghost" :loading="removing" @click="remove" />
            </div>
          </div>

          <div v-else class="flex items-center gap-3 rounded-xl bg-elevated/60 p-4">
            <UIcon name="i-lucide-file-key-2" class="size-8 shrink-0 text-muted" />
            <div>
              <p class="font-semibold text-highlighted">
                Nenhum certificado enviado
              </p>
              <p class="text-sm text-muted">
                Sem ele o sistema não emite notas fiscais.
              </p>
            </div>
          </div>
        </UPageCard>

        <!-- Enviar / trocar -->
        <UPageCard
          v-if="showForm"
          :title="current ? 'Trocar certificado' : 'Enviar certificado'"
          description="Certificado A1 (arquivo .pfx ou .p12) e a senha dele. O arquivo fica guardado criptografado no servidor do box e a senha não aparece mais depois de enviada."
        >
          <form class="space-y-4" @submit.prevent="send">
            <UFileUpload
              v-model="file"
              accept=".pfx,.p12,application/x-pkcs12"
              label="Arraste o arquivo aqui ou clique para escolher"
              description="Arquivo .pfx ou .p12"
              icon="i-lucide-file-key-2"
              layout="list"
              :disabled="!keyConfigured"
              class="w-full"
              :ui="{ base: 'min-h-36' }"
            />
            <UFormField label="Senha do certificado" name="password">
              <UInput
                v-model="password"
                :type="passwordVisible ? 'text' : 'password'"
                autocomplete="new-password"
                :disabled="!keyConfigured"
                class="w-full sm:max-w-sm"
              >
                <template #trailing>
                  <UButton
                    color="neutral"
                    variant="link"
                    :icon="passwordVisible ? 'i-lucide-eye-off' : 'i-lucide-eye'"
                    :aria-label="passwordVisible ? 'Esconder a senha' : 'Mostrar a senha'"
                    :aria-pressed="passwordVisible"
                    @click="passwordVisible = !passwordVisible"
                  />
                </template>
              </UInput>
            </UFormField>

            <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />

            <div class="flex flex-wrap gap-2">
              <UButton
                type="submit"
                :label="current ? 'Trocar' : 'Enviar'"
                icon="i-lucide-upload"
                :loading="sending"
                :disabled="!keyConfigured"
              />
              <UButton v-if="current" label="Cancelar" color="neutral" variant="ghost" @click="cancelReplace" />
            </div>
          </form>
        </UPageCard>

        <!-- Anteriores -->
        <UPageCard v-if="history.length" title="Certificados anteriores" description="Só os dados: o arquivo e a senha de um certificado substituído são apagados.">
          <ul class="divide-y divide-default">
            <li v-for="h in history" :key="h.id" class="flex flex-col gap-1 py-3 sm:flex-row sm:items-center sm:justify-between">
              <div class="min-w-0">
                <p class="font-medium text-highlighted break-words">
                  {{ h.holderName }}
                </p>
                <p class="text-sm text-muted">
                  Validade até {{ formatDate(h.notAfter) }} · enviado em {{ formatDate(h.uploadedAt) }}<template v-if="h.uploadedBy">
                    por {{ h.uploadedBy.name }}
                  </template>
                </p>
              </div>
              <p v-if="h.replacedAt" class="text-sm text-muted sm:text-right">
                Saiu de uso em {{ formatDate(h.replacedAt) }}
              </p>
            </li>
          </ul>
        </UPageCard>
      </div>
    </template>
  </UDashboardPanel>
</template>
