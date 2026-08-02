<template>
  <el-button text class="text-button" @click="emit('edit')">
    编辑
  </el-button>
  <el-button text class="text-button" @click="emit('group')">
    分组
  </el-button>
  <el-dropdown trigger="click" @command="command => emit('cache', command)">
    <el-button text class="text-button">
      <span v-if="caching">
        <el-icon class="is-loading"><Loading /></el-icon> 缓存中
      </span>
      <span v-else>
        缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
      </span>
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item v-if="!isLocalBook" command="cacheBook">
          缓存到服务器
        </el-dropdown-item>
        <el-dropdown-item command="cacheBookLocal">
          缓存到浏览器
        </el-dropdown-item>
        <el-dropdown-item v-if="!isLocalBook" command="deleteBookCache">
          删除服务器缓存
        </el-dropdown-item>
        <el-dropdown-item command="deleteBookLocalCache">
          删除浏览器缓存
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
  <el-dropdown trigger="click" @command="command => emit('export', command)">
    <el-button text class="text-button">
      导出<el-icon class="el-icon--right"><ArrowDown /></el-icon>
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="txt">导出为TXT</el-dropdown-item>
        <el-dropdown-item command="epub">导出为Epub</el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup>
import { computed } from 'vue'
import { ArrowDown, Loading } from '@element-plus/icons-vue'

const props = defineProps({
  book: {
    type: Object,
    required: true,
  },
  caching: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['edit', 'group', 'cache', 'export'])
const isLocalBook = computed(() => Number(props.book.sourceId || 0) === 0)
</script>

<style scoped>
.text-button {
  padding: 3px 5px;
  margin-left: 0;
}
</style>
