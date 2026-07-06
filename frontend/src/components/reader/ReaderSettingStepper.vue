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
    <output>{{ displayValue }}</output>
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
import { computed } from 'vue'
import {
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
})

const emit = defineEmits(['update:modelValue'])
const canDecrease = computed(() => Number(props.modelValue) > props.min)
const canIncrease = computed(() => Number(props.modelValue) < props.max)
const displayValue = computed(() => (
  readerSettingStepLabel(props.modelValue, props.step)
))

function adjust(direction) {
  emit('update:modelValue', steppedReaderSettingValue({
    value: props.modelValue,
    direction,
    min: props.min,
    max: props.max,
    step: props.step,
  }))
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
.reader-setting-stepper output {
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

.reader-setting-stepper output {
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 750px) {
  .reader-setting-stepper {
    grid-template-columns: 44px minmax(64px, 1fr) 44px;
  }

  .reader-setting-stepper button,
  .reader-setting-stepper output {
    min-height: 42px;
  }
}
</style>
