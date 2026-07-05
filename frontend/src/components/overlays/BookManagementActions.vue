<template>
  <el-button
    :size="buttonSize"
    text
    :class="{ 'text-button': !compact }"
    @click="emit('edit')"
  >
    编辑
  </el-button>
  <el-button
    :size="buttonSize"
    text
    :class="{ 'text-button': !compact }"
    @click="emit('group')"
  >
    分组
  </el-button>
  <el-dropdown @command="command => emit('cache', command)">
    <el-button
      :size="buttonSize"
      text
      :class="{ 'text-button': !compact }"
      :loading="caching"
    >
      缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="cacheBookLocal">
          缓存到浏览器
        </el-dropdown-item>
        <el-dropdown-item v-if="!isLocalBook" command="cacheBook">
          缓存到服务器
        </el-dropdown-item>
        <el-dropdown-item command="deleteBookLocalCache">
          删除浏览器缓存
        </el-dropdown-item>
        <el-dropdown-item v-if="!isLocalBook" command="deleteBookCache">
          删除服务器缓存
        </el-dropdown-item>
        <el-dropdown-item v-if="isLocalBook && !compact" disabled>
          本地书无需服务器缓存
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
  <el-dropdown @command="command => emit('export', command)">
    <el-button
      :size="buttonSize"
      text
      :class="{ 'text-button': !compact }"
    >
      导出<el-icon class="el-icon--right"><ArrowDown /></el-icon>
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="txt">导出为 TXT</el-dropdown-item>
        <el-dropdown-item command="epub">导出为 Epub</el-dropdown-item>
        <el-dropdown-item command="json">导出书籍数据</el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup>
import { computed } from 'vue'
import { ArrowDown } from '@element-plus/icons-vue'

const props = defineProps({
  book: {
    type: Object,
    required: true,
  },
  caching: {
    type: Boolean,
    default: false,
  },
  compact: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['edit', 'group', 'cache', 'export'])
const isLocalBook = computed(() => Number(props.book.sourceId || 0) === 0)
const buttonSize = computed(() => props.compact ? 'small' : undefined)
</script>

<style scoped>
.text-button {
  padding: 0;
}
</style>
