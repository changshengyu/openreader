<template>
  <div class="reader-setting-stepper">
    <button
      type="button"
      :aria-label="decreaseLabel"
      :disabled="disabled || !canDecrease"
      @click="adjust(-1)"
    >
      −
    </button>
    <button
      v-if="!editing"
      class="reader-setting-stepper-value"
      type="button"
      :aria-label="`${editLabel}：${displayValue}`"
      :disabled="disabled"
      @click="startEditing"
    >
      {{ displayValue }}
    </button>
    <input
      v-else
      ref="inputEl"
      v-model="draft"
      class="reader-setting-stepper-input"
      type="text"
      inputmode="decimal"
      :aria-label="editLabel"
      :disabled="disabled"
      @blur="commit"
      @click.stop
      @keydown.enter.prevent="commit"
      @keydown.escape.prevent="cancel"
    >
    <button
      type="button"
      :aria-label="increaseLabel"
      :disabled="disabled || !canIncrease"
      @click="adjust(1)"
    >
      +
    </button>
  </div>
</template>

<script setup>
import { computed, nextTick, ref } from 'vue'
import {
  normalizeReaderSettingInput,
  readerSettingStepLabel,
  steppedReaderSettingValue,
} from '../../utils/readerSettingStepper'

const props = defineProps({
  modelValue: { type: Number, required: true },
  min: { type: Number, required: true },
  max: { type: Number, required: true },
  step: { type: Number, default: 1 },
  disabled: { type: Boolean, default: false },
  decreaseLabel: { type: String, default: '减小' },
  increaseLabel: { type: String, default: '增大' },
  editLabel: { type: String, default: '自定义数值' },
})

const emit = defineEmits(['update:modelValue'])
const canDecrease = computed(() => Number(props.modelValue) > props.min)
const canIncrease = computed(() => Number(props.modelValue) < props.max)
const displayValue = computed(() => (
  readerSettingStepLabel(props.modelValue, props.step)
))
const editing = ref(false)
const draft = ref('')
const inputEl = ref(null)

function adjust(direction) {
  emit('update:modelValue', steppedReaderSettingValue({
    value: props.modelValue,
    direction,
    min: props.min,
    max: props.max,
    step: props.step,
  }))
}

function startEditing() {
  if (props.disabled) return
  draft.value = displayValue.value
  editing.value = true
  nextTick(() => {
    inputEl.value?.focus()
    inputEl.value?.select()
  })
}

function commit() {
  if (!editing.value) return
  const next = normalizeReaderSettingInput({
    input: draft.value,
    fallback: props.modelValue,
    min: props.min,
    max: props.max,
  })
  editing.value = false
  if (next !== Number(props.modelValue)) emit('update:modelValue', next)
}

function cancel() {
  editing.value = false
  draft.value = displayValue.value
}
</script>

<style scoped>
.reader-setting-stepper {
  display: grid;
  width: 100%;
  min-width: 0;
  grid-template-columns: 48px minmax(72px, 1fr) 48px;
  color: #5f564a;
  background: rgba(255, 255, 255, 0.52);
  border: 1px solid #e3dccf;
  border-radius: 4px;
  overflow: hidden;
}

.reader-setting-stepper button,
.reader-setting-stepper input {
  display: grid;
  min-height: 36px;
  place-items: center;
  color: inherit;
  background: transparent;
  border: 0;
  font: inherit;
}

.reader-setting-stepper button {
  cursor: pointer;
  font-size: 19px;
}

.reader-setting-stepper button:first-child {
  border-right: 1px solid #e3dccf;
}

.reader-setting-stepper button:last-child {
  border-left: 1px solid #e3dccf;
}

.reader-setting-stepper button:hover:not(:disabled) {
  color: #ed4259;
  background: rgba(237, 66, 89, 0.07);
}

.reader-setting-stepper button:disabled {
  color: #b8b0a4;
  cursor: default;
}

.reader-setting-stepper-value,
.reader-setting-stepper-input {
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

.reader-setting-stepper-value {
  cursor: text !important;
}

.reader-setting-stepper-input {
  width: 100%;
  min-width: 0;
  padding: 0 8px;
  text-align: center;
  outline: 2px solid rgba(64, 158, 255, 0.42);
  outline-offset: -2px;
}

@media (max-width: 750px) {
  .reader-setting-stepper {
    grid-template-columns: 44px minmax(64px, 1fr) 44px;
  }

  .reader-setting-stepper button,
  .reader-setting-stepper input {
    min-height: 42px;
  }
}
</style>
