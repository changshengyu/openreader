<template>
  <BookInfoDialog
    v-model="overlay.bookInfoVisible"
    :book="overlay.bookInfoBook"
    :source-name="bookInfoSourceName"
    :category-name="bookInfoCategory"
    :progress="bookInfoProgress"
    :chapters="overlay.bookInfoBook?.chapterCount || 0"
    :status-label="overlay.bookInfoOptions.statusLabel || sourceStatusLabel"
    :status-type="overlay.bookInfoOptions.statusType || 'info'"
    :cover-editable="!!overlay.bookInfoBook?.id"
    :cover-uploading="coverUploadingBookId === overlay.bookInfoBook?.id"
    :show-update-switch="!!overlay.bookInfoBook?.id && Number(overlay.bookInfoBook?.sourceId || 0) > 0"
    :can-update="overlay.bookInfoBook?.canUpdate !== false"
    :update-switch-loading="updatingBookId === overlay.bookInfoBook?.id"
    @cover-upload="uploadBookInfoCover"
    @can-update-change="toggleBookCanUpdate"
  >
    <div v-if="overlay.bookInfoOptions.actions?.length" class="overlay-actions">
      <el-button
        v-for="action in overlay.bookInfoOptions.actions"
        :key="action.label"
        :type="action.type || 'default'"
        :plain="action.plain"
        :loading="!!action.loading"
        :disabled="!!action.disabled"
        @click="action.handler?.(overlay.bookInfoBook)"
      >
        {{ action.label }}
      </el-button>
    </div>
    <div v-else-if="overlay.bookInfoBook?.id" class="overlay-actions">
      <el-button type="primary" @click="continueRead(overlay.bookInfoBook)">继续阅读</el-button>
      <el-button plain @click="openContentSearch(overlay.bookInfoBook)">搜正文</el-button>
      <el-button plain @click="openBookmarks(overlay.bookInfoBook)">书签</el-button>
      <el-button plain @click="setBookGroup(overlay.bookInfoBook)">设置分组</el-button>
      <el-button plain :loading="refreshingBookId === overlay.bookInfoBook.id" @click="refreshBookInfo(overlay.bookInfoBook)">刷新目录</el-button>
      <el-button v-if="Number(overlay.bookInfoBook.sourceId || 0) > 0" plain :loading="cachingBookId === overlay.bookInfoBook.id" @click="cacheBook(overlay.bookInfoBook, 'cacheBook')">缓存</el-button>
      <el-button plain @click="goDetail(overlay.bookInfoBook)">详情</el-button>
      <el-button plain :loading="loadingUpdates" @click="refreshShelf">刷新书架</el-button>
      <el-button plain type="danger" :loading="deletingBookId === overlay.bookInfoBook.id" @click="deleteBookFromInfo(overlay.bookInfoBook)">移出书架</el-button>
    </div>
  </BookInfoDialog>

  <el-dialog
    v-model="overlay.importBookVisible"
    title="导入本地书籍"
    width="520px"
    class="import-book-dialog"
    :fullscreen="isMobileOverlay"
    @open="loadImportCategories"
  >
    <div class="import-form">
      <el-upload drag :show-file-list="false" :auto-upload="false" accept=".txt,.text,.md,.epub,.pdf,.umd" @change="pickImportFile">
        <el-icon class="upload-icon"><UploadFilled /></el-icon>
        <div class="upload-text">{{ importDraft.file ? importDraft.file.name : '拖入或选择 TXT / EPUB / PDF / UMD 文件' }}</div>
      </el-upload>
      <el-input v-model="importDraft.title" placeholder="书名（可选，不填则使用文件名）" />
      <el-input v-model="importDraft.author" placeholder="作者（可选）" />
      <el-select v-model="importDraft.categoryId" placeholder="分组（可选）" clearable>
        <el-option label="未分组" value="" />
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
      </el-select>
    </div>
    <template #footer>
      <el-button @click="overlay.importBookVisible = false">取消</el-button>
      <el-button type="primary" :loading="importingBook" :disabled="!importDraft.file" @click="importLocalBook">导入</el-button>
    </template>
  </el-dialog>

  <el-drawer
    v-model="overlay.bookManageVisible"
    title="书架管理"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-manage-drawer"
  >
    <el-table
      :data="managedBooks"
      row-key="id"
      height="calc(100vh - 188px)"
      class="manage-table desktop-manage-table"
      @selection-change="onManageSelectionChange"
    >
      <el-table-column type="selection" width="42" />
      <el-table-column prop="title" label="书名" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <el-button text class="text-button" @click="overlay.openBookInfo(row)">{{ row.title }}</el-button>
        </template>
      </el-table-column>
      <el-table-column prop="author" label="作者" min-width="120" show-overflow-tooltip />
      <el-table-column label="分组" min-width="120">
        <template #default="{ row }">{{ categoryName(row.categoryId) }}</template>
      </el-table-column>
      <el-table-column label="章节" min-width="150">
        <template #default="{ row }">
          <span>共 {{ row.chapterCount || 0 }} 章</span><br>
          <span>阅读进度：{{ progressLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button text class="text-button" @click="goDetail(row)">编辑</el-button>
          <el-button text class="text-button" @click="setBookGroup(row)">分组</el-button>
          <el-dropdown @command="cacheBook(row, $event)">
            <el-button text class="text-button" :loading="cachingBookId === row.id">
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBook">缓存到服务器</el-dropdown-item>
                <el-dropdown-item command="deleteBookCache">删除服务器缓存</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown @command="exportBook(row, $event)">
            <el-button text class="text-button">
              导出<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="json">导出书籍数据</el-dropdown-item>
                <el-dropdown-item disabled>导出为TXT（后端未实现）</el-dropdown-item>
                <el-dropdown-item disabled>导出为Epub（后端未实现）</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>
    <div v-if="managedBooks.length" class="mobile-manage-list">
      <article v-for="book in managedBooks" :key="book.id" class="mobile-manage-card">
        <header>
          <el-checkbox :model-value="selectedBookIds.includes(book.id)" @change="value => toggleManagedBook(book.id, value)" />
          <button type="button" @click="overlay.openBookInfo(book)">
            <strong>{{ book.title }}</strong>
            <span>{{ book.author || '未知作者' }} · {{ categoryName(book.categoryId) }}</span>
          </button>
        </header>
        <p>共 {{ book.chapterCount || 0 }} 章 · 阅读进度 {{ progressLabel(book) }}</p>
        <footer>
          <el-button size="small" text @click="goDetail(book)">编辑</el-button>
          <el-button size="small" text @click="setBookGroup(book)">分组</el-button>
          <el-button size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'cacheBook')">缓存</el-button>
          <el-button size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'deleteBookCache')">清缓存</el-button>
          <el-button size="small" text @click="exportBook(book)">导出</el-button>
        </footer>
      </article>
    </div>
    <div class="manage-footer">
      <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchDeleteBooks">批量删除</el-button>
      <el-dropdown @command="batchAddCategory">
        <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy">
          批量添加分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="category in bookshelf.categories" :key="category.id" :command="category">{{ category.name }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-dropdown @command="batchRemoveCategory">
        <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy">
          批量移除分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="category in bookshelf.categories" :key="category.id" :command="category">{{ category.name }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span class="check-tip">已选择 {{ selectedBookIds.length }} 个</span>
      <el-button :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchCacheBooks">批量缓存</el-button>
      <el-button :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchClearCache">批量清缓存</el-button>
    </div>
  </el-drawer>

  <el-drawer
    v-model="overlay.bookGroupVisible"
    :title="overlay.bookGroupMode === 'set' ? '设置分组' : '分组管理'"
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
  >
    <template v-if="overlay.bookGroupMode === 'set'">
      <el-table :data="bookshelf.categories" row-key="id" class="group-set-table" @row-click="selectBookGroup">
        <el-table-column width="46">
          <template #default="{ row }">
            <span class="radio-cell" :class="{ active: String(settingCategoryId) === String(row.id) }" />
          </template>
        </el-table-column>
        <el-table-column prop="name" label="分组名" />
      </el-table>
      <el-empty v-if="!bookshelf.categories.length" description="还没有自定义分组" />
      <div class="manage-footer group-set-footer">
        <el-button @click="settingCategoryId = ''">未分组</el-button>
        <el-button type="primary" :loading="settingCategorySaving" @click="saveBookGroupSetting">确认</el-button>
        <el-button @click="overlay.bookGroupVisible = false">取消</el-button>
      </div>
    </template>
    <template v-else>
      <div class="group-create">
        <el-input v-model="newGroupName" placeholder="新增分组" size="small" @keyup.enter="createCategory" />
        <el-button size="small" type="primary" :disabled="!newGroupName.trim()" @click="createCategory">新增</el-button>
      </div>
      <div class="group-list">
        <div v-for="category in bookshelf.categories" :key="category.id" class="group-row">
          <span>{{ category.name }}</span>
          <span class="group-actions">
            <el-button size="small" text @click="moveGroup(category, -1)">上移</el-button>
            <el-button size="small" text @click="moveGroup(category, 1)">下移</el-button>
            <el-button size="small" text @click="renameGroup(category)">重命名</el-button>
            <el-button size="small" text type="danger" @click="deleteGroup(category)">删除</el-button>
          </span>
        </div>
      </div>
      <el-empty v-if="!bookshelf.categories.length" description="还没有自定义分组" />
    </template>
  </el-drawer>

  <el-drawer
    v-model="overlay.searchBookContentVisible"
    :title="`搜索正文${overlay.searchBook?.title ? ` · ${overlay.searchBook.title}` : ''}`"
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
    class="global-search-drawer"
  >
    <ReaderSearchPanel
      v-model="contentKeyword"
      :results="contentResults"
      :loading="contentSearching"
      :searched="contentSearched"
      :has-more="contentHasMore"
      :status-text="contentSearchStatus"
      @search="searchCurrentBookContent"
      @load-more="loadMoreCurrentBookContent"
      @jump="jumpToContentResult"
    />
  </el-drawer>

  <el-drawer
    v-model="overlay.bookmarkVisible"
    :title="`书签${overlay.bookmarkBook?.title ? ` · ${overlay.bookmarkBook.title}` : ''}`"
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
    class="global-bookmark-drawer"
  >
    <div v-loading="bookmarkLoading">
      <ReaderBookmarkPanel
        :bookmarks="bookmarkItems"
        :show-add="false"
        @jump="jumpToBookmark"
        @edit="openBookmarkEditor"
        @remove="removeBookmarkItem"
      />
    </div>
  </el-drawer>

  <el-dialog v-model="bookmarkEditorVisible" title="编辑书签" width="380px" :fullscreen="isMobileOverlay">
    <div class="bookmark-editor">
      <el-input v-model="bookmarkDraft.title" placeholder="标题" />
      <el-input v-model="bookmarkDraft.excerpt" type="textarea" :rows="3" placeholder="摘录" />
      <el-input v-model="bookmarkDraft.note" type="textarea" :rows="4" placeholder="笔记" />
    </div>
    <template #footer>
      <el-button @click="bookmarkEditorVisible = false">取消</el-button>
      <el-button type="primary" :loading="bookmarkSaving" @click="saveBookmarkEdit">保存</el-button>
    </template>
  </el-dialog>

  <el-drawer
    v-model="overlay.webdavVisible"
    title="WebDAV"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-file-drawer"
    @open="loadWebDAV"
  >
    <section class="file-overlay">
      <header class="file-overlay-head">
        <div>
          <strong>WebDAV 文件管理</strong>
          <span>{{ webdavPath || '/' }}</span>
        </div>
        <div class="file-actions">
          <el-button size="small" :icon="Refresh" :loading="webdavLoading" @click="loadWebDAV">刷新</el-button>
          <el-button size="small" :icon="FolderOpened" @click="createWebDAVFolder">新建目录</el-button>
          <el-upload :show-file-list="false" :auto-upload="false" @change="uploadWebDAVFile">
            <el-button size="small" :icon="Upload" :loading="webdavUploading">上传</el-button>
          </el-upload>
          <el-button size="small" type="danger" plain :disabled="!webdavSelection.length" @click="deleteSelectedWebDAVItems">
            删除 {{ webdavSelection.length }}
          </el-button>
          <el-button size="small" type="primary" :disabled="!webdavImportSelection.length" :loading="webdavImporting" @click="importSelectedWebDAVBooks">
            加入书架 {{ webdavImportSelection.length }}
          </el-button>
        </div>
      </header>
      <el-breadcrumb separator="/" class="file-breadcrumb">
        <el-breadcrumb-item>
          <button type="button" @click="goWebDAVPath('')">webdav</button>
        </el-breadcrumb-item>
        <el-breadcrumb-item v-for="crumb in webdavBreadcrumbs" :key="crumb.path">
          <button type="button" @click="goWebDAVPath(crumb.path)">{{ crumb.name }}</button>
        </el-breadcrumb-item>
      </el-breadcrumb>
      <el-table
        :data="webdavItems"
        stripe
        v-loading="webdavLoading"
        class="file-table desktop-file-table"
        @selection-change="webdavSelection = $event"
      >
        <el-table-column type="selection" width="42" :selectable="row => !row.isDir" />
        <el-table-column prop="name" label="名称" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <button class="file-name" type="button" @click="openWebDAVItem(row)">
              <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
              <span>{{ row.name }}</span>
            </button>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="90">
          <template #default="{ row }">{{ row.isDir ? '目录' : '文件' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button v-if="!row.isDir && isBackupFile(row)" text type="primary" :loading="webdavRestoring === row.name" @click="restoreWebDAVBackupFile(row)">恢复</el-button>
            <el-button v-if="!row.isDir" text type="primary" @click="downloadWebDAVFile(row)">下载</el-button>
            <el-button v-if="row.importable" text type="primary" :loading="webdavImporting" @click="importWebDAVBook(row)">加入书架</el-button>
            <el-button text @click="renameWebDAVItem(row)">重命名</el-button>
            <el-button text type="danger" @click="deleteWebDAVItem(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="webdavItems.length" v-loading="webdavLoading" class="mobile-file-list">
        <article v-for="row in webdavItems" :key="row.name" class="mobile-file-card">
          <header>
            <button class="mobile-file-name" type="button" @click="openWebDAVItem(row)">
              <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
              <span>{{ row.name }}</span>
            </button>
            <el-checkbox
              v-if="!row.isDir"
              :model-value="webdavSelection.some(item => item.name === row.name)"
              @change="value => toggleWebDAVSelection(row, value)"
            />
          </header>
          <p>{{ joinPath(webdavPath, row.name) }}</p>
          <footer>
            <el-button v-if="!row.isDir && isBackupFile(row)" size="small" text type="primary" :loading="webdavRestoring === row.name" @click="restoreWebDAVBackupFile(row)">恢复</el-button>
            <el-button v-if="!row.isDir" size="small" text type="primary" @click="downloadWebDAVFile(row)">下载</el-button>
            <el-button v-if="row.importable" size="small" text type="primary" :loading="webdavImporting" @click="importWebDAVBook(row)">加入书架</el-button>
            <el-button size="small" text @click="renameWebDAVItem(row)">重命名</el-button>
            <el-button size="small" text type="danger" @click="deleteWebDAVItem(row)">删除</el-button>
          </footer>
        </article>
      </div>
      <el-empty v-if="!webdavLoading && !webdavItems.length" description="WebDAV 目录为空" />
    </section>
  </el-drawer>

  <el-dialog v-model="webdavImportResultDialog" title="WebDAV 导入结果" width="560px" :fullscreen="isMobileOverlay">
    <div class="result-list">
      <div v-for="(item, index) in webdavImportResults" :key="index" class="result-row">
        <el-tag :type="item.book ? 'success' : 'danger'" effect="plain">{{ item.book ? '成功' : '失败' }}</el-tag>
        <span>{{ item.book?.title || item.path }}</span>
        <small>{{ item.error || `${item.book?.chapterCount || 0} 章` }}</small>
      </div>
    </div>
  </el-dialog>

  <el-drawer
    v-model="overlay.backupVisible"
    title="备份恢复"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-backup-drawer"
    @open="loadBackups"
  >
    <section class="backup-overlay">
      <header class="file-overlay-head">
        <div>
          <strong>备份恢复</strong>
          <span>保存当前数据，或从 Legado 备份包恢复</span>
        </div>
        <div class="file-actions">
          <el-button size="small" type="primary" :icon="Upload" :loading="backupLoading" @click="runBackup">保存备份</el-button>
          <el-upload :show-file-list="false" :auto-upload="false" accept=".zip" @change="restoreBackup">
            <el-button size="small" :icon="Refresh" :loading="restoreLoading">恢复 Legado</el-button>
          </el-upload>
          <el-button size="small" :icon="Refresh" :loading="backupListLoading" @click="loadBackups">刷新列表</el-button>
        </div>
      </header>
      <el-table :data="backups" stripe v-loading="backupListLoading" class="desktop-backup-table">
        <el-table-column prop="name" label="文件名" min-width="220" show-overflow-tooltip />
        <el-table-column label="大小" width="110">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column label="时间" width="190">
          <template #default="{ row }">{{ formatDate(row.time) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button text type="primary" @click="downloadBackupFile(row)">下载</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="backups.length" v-loading="backupListLoading" class="mobile-backup-list">
        <article v-for="row in backups" :key="row.name" class="mobile-backup-card">
          <div>
            <strong>{{ row.name }}</strong>
            <span>{{ formatDate(row.time) }} · {{ formatSize(row.size) }}</span>
          </div>
          <el-button size="small" text type="primary" @click="downloadBackupFile(row)">下载</el-button>
        </article>
      </div>
      <el-empty v-if="!backups.length && !backupListLoading" description="暂无备份文件" />
    </section>
  </el-drawer>

  <el-drawer
    v-model="overlay.userManageVisible"
    title="用户管理"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-user-drawer"
    @open="loadUsers"
  >
    <section class="user-overlay">
      <header class="file-overlay-head">
        <div>
          <strong>用户空间</strong>
          <span>管理员可调整书源、书仓权限和用户限制</span>
        </div>
        <div class="file-actions">
          <el-button size="small" :icon="Refresh" :loading="usersLoading" @click="loadUsers">刷新</el-button>
          <el-button size="small" :icon="Delete" :loading="cleanupLoading" @click="cleanupInactive">清理不活跃用户</el-button>
        </div>
      </header>
      <el-table :data="users" stripe v-loading="usersLoading" class="desktop-user-table">
        <el-table-column prop="username" label="用户名" min-width="140" />
        <el-table-column prop="role" label="角色" width="90" />
        <el-table-column prop="bookCount" label="书籍" width="80" />
        <el-table-column prop="sourceCount" label="书源" width="80" />
        <el-table-column label="权限" min-width="300">
          <template #default="{ row }">
            <div class="permission-row">
              <el-switch v-model="row.canEditSources" size="small" active-text="书源" @change="updateUserPermission(row)" />
              <el-switch v-model="row.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(row)" />
            </div>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="users.length" v-loading="usersLoading" class="mobile-user-list">
        <article v-for="user in users" :key="user.id" class="mobile-user-card">
          <header>
            <strong>{{ user.username }}</strong>
            <span>{{ user.role }} · 书籍 {{ user.bookCount || 0 }} · 书源 {{ user.sourceCount || 0 }}</span>
          </header>
          <div class="permission-row">
            <el-switch v-model="user.canEditSources" size="small" active-text="书源" @change="updateUserPermission(user)" />
            <el-switch v-model="user.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(user)" />
          </div>
        </article>
      </div>
      <el-empty v-if="!usersLoading && !users.length" description="暂无用户，或当前账号无管理员权限" />
    </section>
  </el-drawer>

  <el-drawer
    v-model="overlay.replaceRulesVisible"
    title="替换规则"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-replace-drawer"
    @open="loadReplaceRules"
  >
    <section class="replace-overlay">
      <header class="file-overlay-head">
        <div>
          <strong>全局替换规则</strong>
          <span>阅读器会按启用规则处理正文内容</span>
        </div>
        <div class="file-actions">
          <el-button size="small" type="primary" :icon="Edit" @click="openReplaceRuleEditor()">新增规则</el-button>
          <el-button size="small" :icon="Refresh" :loading="replaceRulesLoading" @click="loadReplaceRules">刷新</el-button>
        </div>
      </header>
      <el-table :data="replaceRules" stripe v-loading="replaceRulesLoading" class="desktop-replace-table">
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="pattern" label="匹配" min-width="180" show-overflow-tooltip />
        <el-table-column prop="replacement" label="替换为" min-width="160" show-overflow-tooltip />
        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" size="small" @change="toggleReplaceRule(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button text @click="openReplaceRuleEditor(row)">编辑</el-button>
            <el-button text type="danger" @click="removeReplaceRule(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="replaceRules.length" v-loading="replaceRulesLoading" class="mobile-rule-list">
        <article v-for="rule in replaceRules" :key="rule.id" class="mobile-rule-card">
          <header>
            <div>
              <strong>{{ rule.name || '未命名规则' }}</strong>
              <span>{{ rule.pattern }}</span>
            </div>
            <el-switch v-model="rule.enabled" size="small" @change="toggleReplaceRule(rule)" />
          </header>
          <p>替换为：{{ rule.replacement || '空' }}</p>
          <footer>
            <el-button size="small" text @click="openReplaceRuleEditor(rule)">编辑</el-button>
            <el-button size="small" text type="danger" @click="removeReplaceRule(rule)">删除</el-button>
          </footer>
        </article>
      </div>
      <el-empty v-if="!replaceRulesLoading && !replaceRules.length" description="暂无全局替换规则" />
    </section>
  </el-drawer>

  <el-dialog v-model="replaceRuleDialog" :title="editingReplaceRuleId ? '编辑替换规则' : '新增替换规则'" width="520px" :fullscreen="isMobileOverlay">
    <el-form label-position="top">
      <el-form-item label="名称"><el-input v-model="replaceRuleDraft.name" /></el-form-item>
      <el-form-item label="匹配正则或文本"><el-input v-model="replaceRuleDraft.pattern" /></el-form-item>
      <el-form-item label="替换为"><el-input v-model="replaceRuleDraft.replacement" /></el-form-item>
      <el-form-item><el-switch v-model="replaceRuleDraft.enabled" active-text="启用" inactive-text="停用" /></el-form-item>
      <el-form-item label="测试文本">
        <el-input v-model="replaceRuleTestText" type="textarea" :rows="3" />
      </el-form-item>
      <div class="replace-test-actions">
        <el-button size="small" :loading="replaceRuleTesting" @click="runReplaceRuleTest">测试规则</el-button>
        <span v-if="replaceRuleTestResult" :class="replaceRuleTestResult.changed ? 'msg-success' : 'msg-muted'">
          {{ replaceRuleTestResult.changed ? '已发生替换' : '未匹配' }}
        </span>
      </div>
      <pre v-if="replaceRuleTestResult" class="replace-test-output">{{ replaceRuleTestResult.output }}</pre>
    </el-form>
    <template #footer>
      <el-button @click="replaceRuleDialog = false">取消</el-button>
      <el-button type="primary" :loading="replaceRuleSaving" @click="saveReplaceRule">保存</el-button>
    </template>
  </el-dialog>

  <el-drawer
    v-model="overlay.rssVisible"
    title="RSS"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-rss-drawer"
    @open="openRSSOverlay"
  >
    <section class="rss-overlay-grid">
      <article class="rss-overlay-panel">
        <header class="rss-overlay-head">
          <div>
            <strong>RSS 源</strong>
            <span>{{ rssSources.length }} 个订阅</span>
          </div>
          <div class="rss-actions">
            <el-button size="small" type="primary" @click="openRSSEditor()">新增</el-button>
            <el-button size="small" :loading="rssSourcesLoading" @click="loadRSSSources">刷新</el-button>
          </div>
        </header>
        <div v-loading="rssSourcesLoading" class="rss-source-list">
          <div
            v-for="source in rssSources"
            :key="source.id"
            class="rss-source-row"
            :class="{ active: selectedRSSSourceId === source.id }"
          >
            <button type="button" @click="selectRSSSource(source.id)">
              <strong>{{ source.title || '未命名 RSS' }}</strong>
              <small>{{ source.url }}</small>
            </button>
            <span class="rss-source-tools">
              <el-tag size="small" :type="source.enabled === false ? 'info' : 'success'" effect="plain">
                {{ source.enabled === false ? '停用' : '启用' }}
              </el-tag>
              <el-button size="small" text :loading="rssRefreshing === source.id" @click="refreshRSS(source)">刷新</el-button>
              <el-button size="small" text @click="openRSSEditor(source)">编辑</el-button>
              <el-button size="small" text type="danger" @click="removeRSSSource(source)">删除</el-button>
            </span>
          </div>
          <el-empty v-if="!rssSourcesLoading && !rssSources.length" description="暂无 RSS 源" />
        </div>
      </article>

      <article class="rss-overlay-panel">
        <header class="rss-overlay-head">
          <div>
            <strong>文章</strong>
            <span>{{ rssArticles.length }} 篇</span>
          </div>
          <div class="rss-actions">
            <el-radio-group v-model="rssArticleFilter" size="small" @change="loadRSSArticles">
              <el-radio-button value="all">全部</el-radio-button>
              <el-radio-button value="unread">未读</el-radio-button>
              <el-radio-button value="favorite">收藏</el-radio-button>
            </el-radio-group>
            <el-button size="small" :loading="rssArticlesLoading" @click="loadRSSArticles">刷新文章</el-button>
          </div>
        </header>
        <div v-loading="rssArticlesLoading" class="rss-article-list">
          <article v-for="article in rssArticles" :key="article.id" class="rss-article-row" :class="{ read: article.isRead }">
            <button type="button" @click="openRSSArticle(article)">
              <strong>{{ article.title }}</strong>
              <small>{{ formatDate(article.publishedAt || article.updatedAt) }} · {{ article.author || '未知作者' }}</small>
              <span>{{ stripHTML(article.summary || article.content || '无摘要') }}</span>
            </button>
            <el-button
              size="small"
              text
              :type="article.favorite ? 'warning' : 'info'"
              @click="toggleRSSFavorite(article)"
            >
              {{ article.favorite ? '已收藏' : '收藏' }}
            </el-button>
          </article>
          <el-empty v-if="!rssArticlesLoading && !rssArticles.length" description="暂无 RSS 文章" />
        </div>
      </article>
    </section>
  </el-drawer>

  <el-dialog v-model="rssDialog" :title="editingRSSSourceId ? '编辑 RSS 源' : '新增 RSS 源'" width="520px" :fullscreen="isMobileOverlay">
    <el-form label-position="top">
      <el-form-item label="名称"><el-input v-model="rssDraft.title" /></el-form-item>
      <el-form-item label="订阅地址"><el-input v-model="rssDraft.url" /></el-form-item>
      <el-form-item><el-switch v-model="rssDraft.enabled" active-text="启用" inactive-text="停用" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="rssDialog = false">取消</el-button>
      <el-button type="primary" :loading="rssSaving" @click="saveRSSSource">保存</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="rssArticleDialog" title="RSS 文章" width="720px" class="rss-reader-dialog" :fullscreen="isMobileOverlay">
    <article v-if="selectedRSSArticle" class="rss-reader">
      <h2>{{ selectedRSSArticle.title }}</h2>
      <small>{{ formatDate(selectedRSSArticle.publishedAt || selectedRSSArticle.updatedAt) }} · {{ selectedRSSArticle.author || '未知作者' }}</small>
      <p>{{ stripHTML(selectedRSSArticle.content || selectedRSSArticle.summary || '无正文内容') }}</p>
    </article>
    <template #footer>
      <el-button @click="rssArticleDialog = false">关闭</el-button>
      <el-button v-if="selectedRSSArticle?.link" type="primary" @click="openExternal(selectedRSSArticle.link)">打开原文</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Delete, Document, Edit, FolderOpened, Refresh, Upload, UploadFilled } from '@element-plus/icons-vue'
import { cleanupInactiveUsers, listUsers, updateUser } from '../api/admin'
import { cacheBookContent, checkBookUpdates, deleteBookmark, listBookmarks, refreshBook, searchBookContent, updateBook, updateBookCategory, updateBookmark } from '../api/books'
import { downloadBackup, listBackups, restoreLegadoBackup, restoreWebDAVBackup, triggerBackup } from '../api/backup'
import { createReplaceRule, deleteReplaceRule, listReplaceRules, testReplaceRule, updateReplaceRule } from '../api/replaceRules'
import { createRSSSource, deleteRSSSource, listRSSArticles, listRSSSources, refreshRSSSource, updateRSSArticle, updateRSSSource } from '../api/rss'
import { listSources } from '../api/sources'
import { uploadAsset } from '../api/uploads'
import { createWebDAVDirectory, deleteWebDAV, downloadWebDAV, importFromWebDAV, listWebDAV, renameWebDAV, uploadWebDAV } from '../api/webdav'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { compareByShelfOrder } from '../utils/bookOrder'
import BookInfoDialog from './BookInfoDialog.vue'
import ReaderBookmarkPanel from './reader/ReaderBookmarkPanel.vue'
import ReaderSearchPanel from './reader/ReaderSearchPanel.vue'

const router = useRouter()
const route = useRoute()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()

const selectedBookIds = ref([])
const batchBusy = ref(false)
const cachingBookId = ref(null)
const refreshingBookId = ref(null)
const coverUploadingBookId = ref(null)
const updatingBookId = ref(null)
const deletingBookId = ref(null)
const settingCategoryId = ref('')
const settingCategorySaving = ref(false)
const loadingUpdates = ref(false)
const importingBook = ref(false)
const newGroupName = ref('')
const importDraft = reactive({ title: '', author: '', categoryId: '', file: null })
const sourceRows = ref([])
const contentKeyword = ref('')
const contentResults = ref([])
const contentSearching = ref(false)
const contentSearched = ref(false)
const contentLastIndex = ref(-1)
const contentHasMore = ref(false)
const contentTotal = ref(0)
const bookmarkItems = ref([])
const bookmarkLoading = ref(false)
const bookmarkEditorVisible = ref(false)
const bookmarkSaving = ref(false)
const editingBookmark = ref(null)
const bookmarkDraft = reactive({ title: '', excerpt: '', note: '' })
const rssSources = ref([])
const rssArticles = ref([])
const selectedRSSSourceId = ref('')
const rssSourcesLoading = ref(false)
const rssArticlesLoading = ref(false)
const rssRefreshing = ref(null)
const rssDialog = ref(false)
const rssSaving = ref(false)
const editingRSSSourceId = ref(null)
const rssDraft = ref({ title: '', url: '', enabled: true })
const rssArticleDialog = ref(false)
const selectedRSSArticle = ref(null)
const rssArticleFilter = ref('all')
const webdavPath = ref('')
const webdavItems = ref([])
const webdavSelection = ref([])
const webdavLoading = ref(false)
const webdavUploading = ref(false)
const webdavRestoring = ref('')
const webdavImporting = ref(false)
const webdavImportResultDialog = ref(false)
const webdavImportResults = ref([])
const backups = ref([])
const backupLoading = ref(false)
const backupListLoading = ref(false)
const restoreLoading = ref(false)
const users = ref([])
const usersLoading = ref(false)
const cleanupLoading = ref(false)
const replaceRules = ref([])
const replaceRulesLoading = ref(false)
const replaceRuleDialog = ref(false)
const replaceRuleSaving = ref(false)
const replaceRuleTesting = ref(false)
const editingReplaceRuleId = ref(null)
const replaceRuleDraft = ref({ name: '', pattern: '', replacement: '', enabled: true })
const replaceRuleTestText = ref('广告123\n正文内容')
const replaceRuleTestResult = ref(null)
const windowWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
const coarsePointer = ref(typeof window === 'undefined' ? false : window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false)

const isMobileOverlay = computed(() => windowWidth.value <= 860 || coarsePointer.value)
const wideDrawerDirection = computed(() => isMobileOverlay.value ? 'btt' : 'rtl')
const wideDrawerSize = computed(() => isMobileOverlay.value ? '88%' : '82%')
const narrowDrawerDirection = computed(() => isMobileOverlay.value ? 'btt' : 'rtl')
const narrowDrawerSize = computed(() => isMobileOverlay.value ? '86%' : '420px')
const bookInfoCategory = computed(() => overlay.bookInfoOptions.categoryName || categoryName(overlay.bookInfoBook?.categoryId))
const bookInfoSourceName = computed(() => {
  if (overlay.bookInfoOptions.sourceName) return overlay.bookInfoOptions.sourceName
  const sourceId = overlay.bookInfoBook?.sourceId
  if (!sourceId) return '本地'
  return sourceRows.value.find(source => Number(source.id) === Number(sourceId))?.name || '远程书籍'
})
const bookInfoProgress = computed(() => {
  const book = overlay.bookInfoBook
  return book ? (reader.progressByBook[book.id]?.percent || book.progress?.percent || 0) : 0
})
const sourceStatusLabel = computed(() => overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍')
const managedBooks = computed(() => [...bookshelf.books].sort(compareByShelfOrder))
const contentSearchStatus = computed(() => {
  if (!contentSearched.value) return ''
  const scanned = contentLastIndex.value >= 0 ? contentLastIndex.value + 1 : 0
  if (!contentTotal.value) return `${contentResults.value.length} 条结果`
  return `已搜索 ${Math.min(scanned, contentTotal.value)} / ${contentTotal.value} 章，${contentResults.value.length} 条结果`
})
const webdavBreadcrumbs = computed(() => {
  if (!webdavPath.value) return []
  const parts = webdavPath.value.split('/').filter(Boolean)
  return parts.map((name, index) => ({ name, path: parts.slice(0, index + 1).join('/') }))
})
const webdavImportSelection = computed(() => webdavSelection.value.filter(row => row.importable))

onMounted(() => {
  window.addEventListener('resize', updateWindowWidth, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateWindowWidth)
})

function updateWindowWidth() {
  windowWidth.value = window.innerWidth
  coarsePointer.value = window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false
}

async function loadImportCategories() {
  try {
    if (!bookshelf.categories.length) await bookshelf.loadCategories()
  } catch (err) {
    ElMessage.error(readError(err, '加载分组失败'))
  }
}

function pickImportFile(data) {
  importDraft.file = data.raw || null
  if (importDraft.file && !importDraft.title) {
    importDraft.title = importDraft.file.name.replace(/\.[^.]+$/, '')
  }
}

async function importLocalBook() {
  if (!importDraft.file) return
  importingBook.value = true
  try {
    const book = await bookshelf.importTXT({
      file: importDraft.file,
      title: importDraft.title,
      author: importDraft.author,
      categoryId: importDraft.categoryId,
    })
    ElMessage.success(`已导入《${book.title}》，共 ${book.chapterCount || 0} 章`)
    Object.assign(importDraft, { title: '', author: '', categoryId: '', file: null })
    overlay.importBookVisible = false
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importingBook.value = false
  }
}

watch(
  () => overlay.bookManageVisible || overlay.bookGroupVisible,
  async (visible) => {
    if (!visible) return
    try {
      await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()])
      if (overlay.bookGroupVisible && overlay.bookGroupMode === 'set') {
        settingCategoryId.value = overlay.bookInfoBook?.categoryId ? String(overlay.bookInfoBook.categoryId) : ''
      }
    } catch (err) {
      ElMessage.error(readError(err, '加载书架数据失败'))
    }
  },
)

watch(
  () => overlay.bookInfoVisible,
  async (visible) => {
    if (!visible) return
    try {
      await bookshelf.loadCategories()
      if (overlay.bookInfoBook?.sourceId && !sourceRows.value.length) {
        await loadSourceRows()
      }
    } catch (err) {
      ElMessage.error(readError(err, '加载书籍信息失败'))
    }
  },
)

watch(
  () => overlay.searchBookContentVisible,
  (visible) => {
    if (visible) return
    contentKeyword.value = ''
    contentResults.value = []
    contentSearched.value = false
    contentLastIndex.value = -1
    contentHasMore.value = false
    contentTotal.value = 0
  },
)

watch(contentKeyword, () => {
  contentResults.value = []
  contentSearched.value = false
  contentLastIndex.value = -1
  contentHasMore.value = false
  contentTotal.value = 0
})

watch(
  () => overlay.bookmarkVisible,
  async (visible) => {
    if (!visible) {
      bookmarkItems.value = []
      return
    }
    await loadBookmarkItems()
  },
)

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => String(category.id) === String(id))?.name || '未分组'
}

function progressLabel(book) {
  const progress = reader.progressByBook[book.id] || book.progress
  return `${Math.round((progress?.percent || 0) * 100)}%`
}

async function loadSourceRows() {
  const { data } = await listSources()
  sourceRows.value = data || []
}

function onManageSelectionChange(rows) {
  selectedBookIds.value = rows.map(row => row.id)
}

function toggleManagedBook(bookId, checked) {
  if (checked) {
    if (!selectedBookIds.value.includes(bookId)) selectedBookIds.value.push(bookId)
    return
  }
  selectedBookIds.value = selectedBookIds.value.filter(id => id !== bookId)
}

function continueRead(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id } })
}

function goDetail(book) {
  overlay.closeBookInfo()
  overlay.bookManageVisible = false
  router.push({ name: 'book-detail', params: { id: book.id } })
}

function setBookGroup(book) {
  overlay.openBookGroup('set', book, {
    categoryName: categoryName(book.categoryId),
    progress: (reader.progressByBook[book.id]?.percent || book.progress?.percent || 0),
  })
}

function selectBookGroup(category) {
  settingCategoryId.value = String(category.id)
}

async function saveBookGroupSetting() {
  const book = overlay.bookInfoBook
  if (!book?.id) return
  settingCategorySaving.value = true
  try {
    const categoryId = settingCategoryId.value ? Number(settingCategoryId.value) : null
    const { data } = await updateBookCategory(book.id, categoryId)
    bookshelf.upsertBook(data)
    overlay.bookInfoBook = data
    overlay.bookInfoOptions = {
      ...overlay.bookInfoOptions,
      categoryName: categoryName(data.categoryId),
      progress: reader.progressByBook[data.id]?.percent || data.progress?.percent || 0,
    }
    overlay.bookGroupVisible = false
    ElMessage.success('分组已设置')
  } catch (err) {
    ElMessage.error(readError(err, '设置分组失败'))
  } finally {
    settingCategorySaving.value = false
  }
}

function openContentSearch(book) {
  overlay.closeBookInfo()
  overlay.openSearchBookContent(book)
}

function openBookmarks(book) {
  overlay.closeBookInfo()
  overlay.openBookmark(book)
}

async function refreshShelf() {
  loadingUpdates.value = true
  try {
    const { data } = await checkBookUpdates()
    await bookshelf.loadBooks()
    ElMessage.success(data?.newChapters ? `发现 ${data.newChapters} 个新章节` : '暂未发现新章节')
  } catch (err) {
    ElMessage.error(readError(err, '刷新失败'))
  } finally {
    loadingUpdates.value = false
  }
}

async function refreshBookInfo(book) {
  if (!book?.id) return
  refreshingBookId.value = book.id
  try {
    const { data } = await refreshBook(book.id)
    const updatedBook = data?.book || data
    if (updatedBook?.id) {
      bookshelf.upsertBook(updatedBook)
      overlay.bookInfoBook = updatedBook
    } else {
      await bookshelf.loadBooks()
    }
    ElMessage.success(`目录已刷新，共 ${data?.chapterCount || updatedBook?.chapterCount || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '刷新目录失败'))
  } finally {
    refreshingBookId.value = null
  }
}

async function deleteBookFromInfo(book) {
  if (!book?.id) return
  deletingBookId.value = book.id
  try {
    await ElMessageBox.confirm(`确定将《${book.title || '这本书'}》移出书架吗？`, '移出书架', { type: 'warning' })
    await bookshelf.removeBook(book.id)
    overlay.closeBookInfo()
    ElMessage.success('已移出书架')
    const currentBookId = Number(route.params.id || 0)
    if ((route.name === 'reader' || route.name === 'book-detail') && currentBookId === Number(book.id)) {
      await router.push({ name: 'home' })
    }
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '移出书架失败'))
  } finally {
    deletingBookId.value = null
  }
}

async function uploadBookInfoCover(file) {
  const book = overlay.bookInfoBook
  if (!book?.id || !file) return
  coverUploadingBookId.value = book.id
  try {
    const { data: uploadResult } = await uploadAsset({ file, type: 'cover' })
    const { data: updatedBook } = await updateBook(book.id, {
      title: book.title,
      author: book.author || '',
      coverUrl: uploadResult.url,
      intro: book.intro || '',
      categoryId: book.categoryId || null,
      canUpdate: book.canUpdate !== false,
    })
    bookshelf.upsertBook(updatedBook)
    overlay.bookInfoBook = updatedBook
    ElMessage.success('封面已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新封面失败'))
  } finally {
    coverUploadingBookId.value = null
  }
}

async function toggleBookCanUpdate(value) {
  const book = overlay.bookInfoBook
  if (!book?.id || !book.sourceId) return
  updatingBookId.value = book.id
  try {
    const { data: updatedBook } = await updateBook(book.id, {
      title: book.title,
      author: book.author || '',
      coverUrl: book.coverUrl || '',
      intro: book.intro || '',
      categoryId: book.categoryId || null,
      canUpdate: value,
    })
    bookshelf.upsertBook(updatedBook)
    overlay.bookInfoBook = updatedBook
    ElMessage.success(value ? '已开启追更' : '已关闭追更')
  } catch (err) {
    ElMessage.error(readError(err, '更新追更状态失败'))
  } finally {
    updatingBookId.value = null
  }
}

async function batchAddCategory(category) {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    await bookshelf.batchSetCategory([...selectedBookIds.value], category.id)
    ElMessage.success(`已添加到“${category.name}”分组`)
  } catch (err) {
    ElMessage.error(readError(err, '批量添加分组失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchRemoveCategory(category) {
  if (!selectedBookIds.value.length) return
  const targetIds = managedBooks.value
    .filter(book => selectedBookIds.value.includes(book.id) && String(book.categoryId) === String(category.id))
    .map(book => book.id)
  if (!targetIds.length) {
    ElMessage.info('选中书籍不在该分组中')
    return
  }
  batchBusy.value = true
  try {
    await bookshelf.batchSetCategory(targetIds, null)
    ElMessage.success(`已从“${category.name}”分组移除`)
  } catch (err) {
    ElMessage.error(readError(err, '批量移除分组失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchCacheBooks() {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    const data = await bookshelf.batchCacheBooks([...selectedBookIds.value])
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '批量缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchClearCache() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定清理选中 ${selectedBookIds.value.length} 本书的章节缓存吗？`, '清理缓存', { type: 'warning' })
    batchBusy.value = true
    const data = await bookshelf.batchClearCache([...selectedBookIds.value])
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchDeleteBooks() {
  if (!selectedBookIds.value.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${selectedBookIds.value.length} 本书吗？`, '批量删除', { type: 'warning' })
    batchBusy.value = true
    await bookshelf.batchDeleteBooks([...selectedBookIds.value])
    selectedBookIds.value = []
    ElMessage.success('已批量删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除失败'))
  } finally {
    batchBusy.value = false
  }
}

async function cacheBook(book, command) {
  if (command === 'deleteBookCache') {
    await clearBookCache(book)
    return
  }
  cachingBookId.value = book.id
  try {
    const { data } = await cacheBookContent(book.id, { all: true })
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
    await bookshelf.loadBooks()
  } catch (err) {
    ElMessage.error(readError(err, '缓存失败'))
  } finally {
    cachingBookId.value = null
  }
}

async function clearBookCache(book) {
  cachingBookId.value = book.id
  try {
    const data = await bookshelf.batchClearCache([book.id])
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    cachingBookId.value = null
  }
}

async function exportBook(book) {
  batchBusy.value = true
  try {
    const blob = await bookshelf.exportSelectedBooks([book.id])
    downloadBlob(blob, `openreader-book-${book.id}.json`)
    ElMessage.success(`已导出《${book.title}》`)
  } catch (err) {
    ElMessage.error(readError(err, '导出失败'))
  } finally {
    batchBusy.value = false
  }
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

async function searchCurrentBookContent() {
  return runCurrentBookContentSearch({ append: false })
}

async function loadMoreCurrentBookContent() {
  return runCurrentBookContentSearch({ append: true })
}

async function runCurrentBookContentSearch({ append = false } = {}) {
  const book = overlay.searchBook
  const keyword = contentKeyword.value.trim()
  if (!book?.id || !keyword) return
  if (contentSearching.value) return
  contentSearching.value = true
  contentSearched.value = true
  try {
    const params = append
      ? {
          paged: 1,
          lastIndex: contentLastIndex.value,
          chapterLimit: 80,
          matchLimit: 200,
          perChapterLimit: 20,
        }
      : {
          paged: 1,
          lastIndex: -1,
          chapterLimit: 80,
          matchLimit: 200,
          perChapterLimit: 20,
        }
    const { data } = await searchBookContent(book.id, keyword, params)
    const rows = Array.isArray(data) ? data : (data?.list || [])
    contentResults.value = append ? contentResults.value.concat(rows) : rows
    contentLastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : -1
    contentHasMore.value = Boolean(data?.hasMore)
    contentTotal.value = Number(data?.total || 0)
  } catch (err) {
    ElMessage.error(readError(err, '搜索正文失败'))
  } finally {
    contentSearching.value = false
  }
}

function jumpToContentResult(result) {
  const book = overlay.searchBook
  if (!book?.id) return
  overlay.searchBookContentVisible = false
  router.push({
    name: 'reader',
    params: { id: book.id },
    query: {
      chapter: Number(result.chapterIndex || 0),
      line: Number.isInteger(result.lineIndex) ? result.lineIndex : undefined,
      q: contentKeyword.value.trim() || undefined,
    },
  })
}

async function loadBookmarkItems() {
  const book = overlay.bookmarkBook
  if (!book?.id) return
  bookmarkLoading.value = true
  try {
    const { data } = await listBookmarks(book.id)
    bookmarkItems.value = data || []
  } catch (err) {
    ElMessage.error(readError(err, '加载书签失败'))
  } finally {
    bookmarkLoading.value = false
  }
}

function jumpToBookmark(bookmark) {
  const book = overlay.bookmarkBook
  if (!book?.id) return
  overlay.bookmarkVisible = false
  router.push({
    name: 'reader',
    params: { id: book.id },
    query: {
      chapter: bookmark.chapterIndex,
      offset: bookmark.offset || 0,
    },
  })
}

function openBookmarkEditor(bookmark) {
  editingBookmark.value = bookmark
  Object.assign(bookmarkDraft, {
    title: bookmark.title || '',
    excerpt: bookmark.excerpt || '',
    note: bookmark.note || '',
  })
  bookmarkEditorVisible.value = true
}

async function saveBookmarkEdit() {
  if (!editingBookmark.value) return
  bookmarkSaving.value = true
  try {
    const { data } = await updateBookmark(editingBookmark.value.id, {
      title: bookmarkDraft.title,
      excerpt: bookmarkDraft.excerpt,
      note: bookmarkDraft.note,
    })
    const index = bookmarkItems.value.findIndex(item => item.id === data.id)
    if (index >= 0) bookmarkItems.value[index] = data
    bookmarkEditorVisible.value = false
    ElMessage.success('书签已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新书签失败'))
  } finally {
    bookmarkSaving.value = false
  }
}

async function removeBookmarkItem(bookmark) {
  try {
    await deleteBookmark(bookmark.id)
    bookmarkItems.value = bookmarkItems.value.filter(item => item.id !== bookmark.id)
    ElMessage.success('书签已删除')
  } catch (err) {
    ElMessage.error(readError(err, '删除书签失败'))
  }
}

async function openRSSOverlay() {
  if (!rssSources.value.length) await loadRSSSources()
  await loadRSSArticles()
}

async function loadRSSSources() {
  rssSourcesLoading.value = true
  try {
    const { data } = await listRSSSources()
    rssSources.value = data || []
    if (!selectedRSSSourceId.value && rssSources.value.length) selectedRSSSourceId.value = rssSources.value[0].id
  } catch (err) {
    ElMessage.error(readError(err, '加载 RSS 源失败'))
  } finally {
    rssSourcesLoading.value = false
  }
}

async function loadRSSArticles() {
  rssArticlesLoading.value = true
  try {
    const params = selectedRSSSourceId.value ? { sourceId: selectedRSSSourceId.value } : {}
    if (rssArticleFilter.value === 'unread') params.unread = true
    if (rssArticleFilter.value === 'favorite') params.favorite = true
    const { data } = await listRSSArticles(params)
    rssArticles.value = data || []
  } catch (err) {
    ElMessage.error(readError(err, '加载 RSS 文章失败'))
  } finally {
    rssArticlesLoading.value = false
  }
}

async function selectRSSSource(sourceId) {
  selectedRSSSourceId.value = sourceId
  await loadRSSArticles()
}

function openRSSEditor(source = null) {
  editingRSSSourceId.value = source?.id || null
  rssDraft.value = {
    title: source?.title || '',
    url: source?.url || '',
    enabled: source?.enabled ?? true,
  }
  rssDialog.value = true
}

async function saveRSSSource() {
  if (!rssDraft.value.url.trim()) {
    ElMessage.warning('RSS 地址不能为空')
    return
  }
  rssSaving.value = true
  try {
    const payload = { ...rssDraft.value, url: rssDraft.value.url.trim() }
    if (editingRSSSourceId.value) {
      await updateRSSSource(editingRSSSourceId.value, payload)
      ElMessage.success('RSS 源已更新')
    } else {
      await createRSSSource(payload)
      ElMessage.success('RSS 源已创建')
    }
    rssDialog.value = false
    await loadRSSSources()
  } catch (err) {
    ElMessage.error(readError(err, '保存 RSS 源失败'))
  } finally {
    rssSaving.value = false
  }
}

async function refreshRSS(source) {
  rssRefreshing.value = source.id
  try {
    const { data } = await refreshRSSSource(source.id)
    ElMessage.success(`已同步 ${data.imported || 0}/${data.total || 0} 篇文章`)
    await loadRSSArticles()
  } catch (err) {
    ElMessage.error(readError(err, '刷新 RSS 源失败'))
  } finally {
    rssRefreshing.value = null
  }
}

async function removeRSSSource(source) {
  try {
    await ElMessageBox.confirm(`确定删除 RSS 源“${source.title}”吗？文章缓存也会删除。`, '删除 RSS 源', { type: 'warning' })
    await deleteRSSSource(source.id)
    rssSources.value = rssSources.value.filter(item => item.id !== source.id)
    if (selectedRSSSourceId.value === source.id) selectedRSSSourceId.value = rssSources.value[0]?.id || ''
    await loadRSSArticles()
    ElMessage.success('RSS 源已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除 RSS 源失败'))
  }
}

async function openRSSArticle(article) {
  selectedRSSArticle.value = article
  rssArticleDialog.value = true
  if (!article.isRead) {
    await updateRSSArticleState(article, { isRead: true }, { silent: true })
  }
}

async function toggleRSSFavorite(article) {
  await updateRSSArticleState(article, { favorite: !article.favorite })
}

async function updateRSSArticleState(article, payload, { silent = false } = {}) {
  try {
    const { data } = await updateRSSArticle(article.id, payload)
    Object.assign(article, data)
    if (selectedRSSArticle.value?.id === article.id) selectedRSSArticle.value = article
    if (!silent) ElMessage.success('文章状态已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新 RSS 文章失败'))
  }
}

function stripHTML(value) {
  return String(value || '')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/p>/gi, '\n\n')
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .trim()
}

function openExternal(url) {
  window.open(url, '_blank', 'noopener,noreferrer')
}

function formatSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

async function loadWebDAV() {
  webdavLoading.value = true
  try {
    const { data } = await listWebDAV(webdavPath.value)
    webdavItems.value = parseWebDAVListing(data)
    webdavSelection.value = []
  } catch (err) {
    ElMessage.error(readError(err, '加载 WebDAV 失败'))
  } finally {
    webdavLoading.value = false
  }
}

async function goWebDAVPath(path) {
  webdavPath.value = path
  await loadWebDAV()
}

function openWebDAVItem(row) {
  if (row.isDir) goWebDAVPath(joinPath(webdavPath.value, row.name))
}

function toggleWebDAVSelection(row, checked) {
  if (checked) {
    if (!webdavSelection.value.some(item => item.name === row.name)) webdavSelection.value.push(row)
    return
  }
  webdavSelection.value = webdavSelection.value.filter(item => item.name !== row.name)
}

async function uploadWebDAVFile(data) {
  const file = data.raw
  if (!file) return
  webdavUploading.value = true
  try {
    await uploadWebDAV({ path: webdavPath.value, file })
    ElMessage.success('WebDAV 文件已上传')
    await loadWebDAV()
  } catch (err) {
    ElMessage.error(readError(err, '上传 WebDAV 失败'))
  } finally {
    webdavUploading.value = false
  }
}

async function createWebDAVFolder() {
  try {
    const { value } = await ElMessageBox.prompt('输入目录名称', '新建 WebDAV 目录', {
      inputValidator: value => !!value?.trim() || '目录名称不能为空',
    })
    await createWebDAVDirectory({ path: webdavPath.value, name: value.trim() })
    ElMessage.success('WebDAV 目录已创建')
    await loadWebDAV()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '创建 WebDAV 目录失败'))
  }
}

async function downloadWebDAVFile(row) {
  try {
    const resp = await downloadWebDAV(joinPath(webdavPath.value, row.name))
    downloadBlob(resp.data, row.name)
  } catch (err) {
    ElMessage.error(readError(err, '下载 WebDAV 文件失败'))
  }
}

function isBackupFile(row) {
  return String(row.name || '').toLowerCase().endsWith('.zip')
}

async function restoreWebDAVBackupFile(row) {
  const path = joinPath(webdavPath.value, row.name)
  try {
    await ElMessageBox.confirm(`确定从 WebDAV 文件“${row.name}”恢复备份吗？`, '恢复 WebDAV 备份', { type: 'warning' })
    webdavRestoring.value = row.name
    const { data } = await restoreWebDAVBackup(path)
    ElMessage.success(`恢复完成：书源 ${data.sources || 0}，书籍 ${data.books || 0}，进度 ${data.progress || 0}`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '恢复 WebDAV 备份失败'))
  } finally {
    webdavRestoring.value = ''
  }
}

async function renameWebDAVItem(row) {
  try {
    const { value } = await ElMessageBox.prompt('输入新的名称', '重命名 WebDAV 项目', {
      inputValue: row.name,
      inputValidator: value => !!value?.trim() || '名称不能为空',
    })
    const name = value.trim()
    if (!name || name === row.name) return
    await renameWebDAV({
      path: joinPath(webdavPath.value, row.name),
      newPath: joinPath(webdavPath.value, name),
    })
    ElMessage.success('已重命名')
    await loadWebDAV()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名 WebDAV 项目失败'))
  }
}

async function deleteWebDAVItem(row) {
  try {
    await ElMessageBox.confirm(`确定删除“${row.name}”吗？`, '删除 WebDAV 项目', { type: 'warning' })
    await deleteWebDAV(joinPath(webdavPath.value, row.name))
    ElMessage.success('已删除')
    await loadWebDAV()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除 WebDAV 项目失败'))
  }
}

async function deleteSelectedWebDAVItems() {
  if (!webdavSelection.value.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${webdavSelection.value.length} 个 WebDAV 项目吗？`, '批量删除 WebDAV 项目', { type: 'warning' })
    for (const row of webdavSelection.value) {
      await deleteWebDAV(joinPath(webdavPath.value, row.name))
    }
    ElMessage.success('已批量删除')
    await loadWebDAV()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除 WebDAV 项目失败'))
  }
}

async function importWebDAVBook(row) {
  if (!row.importable) return
  await importWebDAVBooks([joinPath(webdavPath.value, row.name)])
}

async function importSelectedWebDAVBooks() {
  const paths = webdavImportSelection.value.map(row => joinPath(webdavPath.value, row.name))
  if (paths.length) await importWebDAVBooks(paths)
}

async function importWebDAVBooks(paths) {
  webdavImporting.value = true
  try {
    const { data } = await importFromWebDAV(paths)
    webdavImportResults.value = data.imported || []
    const success = webdavImportResults.value.filter(item => item.book).length
    const failed = webdavImportResults.value.filter(item => item.error).length
    ElMessage.success(`导入 ${success} 本` + (failed ? `，${failed} 本失败` : ''))
    webdavImportResultDialog.value = true
    await bookshelf.loadBooks()
  } catch (err) {
    ElMessage.error(readError(err, '导入 WebDAV 文件失败'))
  } finally {
    webdavImporting.value = false
  }
}

function parseWebDAVListing(xml) {
  const doc = new DOMParser().parseFromString(xml, 'application/xml')
  return [...doc.querySelectorAll('prop')].map((node) => ({
    name: node.querySelector('displayname')?.textContent || '',
    isDir: node.querySelector('iscollection')?.textContent === 'true',
  })).filter(item => item.name && item.name !== webdavPath.value).map(item => ({
    ...item,
    importable: !item.isDir && isImportableBookFile(item.name),
  }))
}

function isImportableBookFile(name) {
  return /\.(txt|text|md|epub|pdf|umd)$/i.test(name || '')
}

function joinPath(base, name) {
  return [base, name].filter(Boolean).join('/')
}

async function runBackup() {
  backupLoading.value = true
  try {
    const { data } = await triggerBackup()
    ElMessage.success(`备份已生成：${data.name || 'backup.zip'}`)
    await loadBackups()
  } catch (err) {
    ElMessage.error(readError(err, '保存备份失败'))
  } finally {
    backupLoading.value = false
  }
}

async function loadBackups() {
  backupListLoading.value = true
  try {
    const { data } = await listBackups()
    backups.value = data || []
  } catch (err) {
    ElMessage.error(readError(err, '加载备份列表失败'))
  } finally {
    backupListLoading.value = false
  }
}

async function downloadBackupFile(row) {
  try {
    const resp = await downloadBackup(row.name)
    downloadBlob(resp.data, row.name)
  } catch (err) {
    ElMessage.error(readError(err, '下载备份失败'))
  }
}

async function restoreBackup(data) {
  const file = data.raw
  if (!file) return
  restoreLoading.value = true
  try {
    const form = new FormData()
    form.append('file', file)
    const { data: result } = await restoreLegadoBackup(form)
    ElMessage.success(`恢复完成：书源 ${result.sources || 0}，书籍 ${result.books || 0}，进度 ${result.progress || 0}`)
    await bookshelf.loadBooks()
  } catch (err) {
    ElMessage.error(readError(err, '恢复备份失败'))
  } finally {
    restoreLoading.value = false
  }
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const { data } = await listUsers()
    users.value = data || []
  } catch (err) {
    ElMessage.error(readError(err, '加载用户失败'))
  } finally {
    usersLoading.value = false
  }
}

async function updateUserPermission(row) {
  try {
    await updateUser(row.id, {
      canEditSources: row.canEditSources,
      canAccessStore: row.canAccessStore,
      bookLimit: row.bookLimit,
      sourceLimit: row.sourceLimit,
    })
    ElMessage.success('用户权限已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新用户失败'))
    await loadUsers()
  }
}

async function cleanupInactive() {
  cleanupLoading.value = true
  try {
    await ElMessageBox.confirm('确定清理不活跃用户吗？', '清理用户', { type: 'warning' })
    await cleanupInactiveUsers()
    ElMessage.success('清理完成')
    await loadUsers()
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') ElMessage.error(readError(err, '清理用户失败'))
  } finally {
    cleanupLoading.value = false
  }
}

async function loadReplaceRules() {
  replaceRulesLoading.value = true
  try {
    const { data } = await listReplaceRules()
    replaceRules.value = data || []
  } catch (err) {
    ElMessage.error(readError(err, '加载替换规则失败'))
  } finally {
    replaceRulesLoading.value = false
  }
}

function openReplaceRuleEditor(rule = null) {
  editingReplaceRuleId.value = rule?.id || null
  replaceRuleDraft.value = {
    name: rule?.name || '',
    pattern: rule?.pattern || '',
    replacement: rule?.replacement || '',
    enabled: rule?.enabled ?? true,
  }
  replaceRuleTestResult.value = null
  replaceRuleDialog.value = true
}

async function saveReplaceRule() {
  if (!replaceRuleDraft.value.pattern.trim()) {
    ElMessage.warning('匹配规则不能为空')
    return
  }
  replaceRuleSaving.value = true
  try {
    const payload = { ...replaceRuleDraft.value, pattern: replaceRuleDraft.value.pattern.trim() }
    if (editingReplaceRuleId.value) {
      await updateReplaceRule(editingReplaceRuleId.value, payload)
      ElMessage.success('替换规则已更新')
    } else {
      await createReplaceRule(payload)
      ElMessage.success('替换规则已创建')
    }
    replaceRuleDialog.value = false
    await loadReplaceRules()
  } catch (err) {
    ElMessage.error(readError(err, '保存替换规则失败'))
  } finally {
    replaceRuleSaving.value = false
  }
}

async function toggleReplaceRule(rule) {
  try {
    await updateReplaceRule(rule.id, {
      name: rule.name,
      pattern: rule.pattern,
      replacement: rule.replacement,
      enabled: rule.enabled,
    })
    ElMessage.success(rule.enabled ? '规则已启用' : '规则已停用')
  } catch (err) {
    ElMessage.error(readError(err, '更新替换规则失败'))
    await loadReplaceRules()
  }
}

async function runReplaceRuleTest() {
  if (!replaceRuleDraft.value.pattern.trim() || !replaceRuleTestText.value) {
    ElMessage.warning('请输入匹配规则和测试文本')
    return
  }
  replaceRuleTesting.value = true
  try {
    const { data } = await testReplaceRule({
      pattern: replaceRuleDraft.value.pattern,
      replacement: replaceRuleDraft.value.replacement,
      text: replaceRuleTestText.value,
    })
    replaceRuleTestResult.value = data
  } catch (err) {
    ElMessage.error(readError(err, '测试替换规则失败'))
  } finally {
    replaceRuleTesting.value = false
  }
}

async function removeReplaceRule(rule) {
  try {
    await ElMessageBox.confirm(`确定删除替换规则“${rule.name || rule.pattern}”吗？`, '删除替换规则', { type: 'warning' })
    await deleteReplaceRule(rule.id)
    replaceRules.value = replaceRules.value.filter(item => item.id !== rule.id)
    ElMessage.success('替换规则已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除替换规则失败'))
  }
}

async function createCategory() {
  const name = newGroupName.value.trim()
  if (!name) return
  try {
    await bookshelf.addCategory({ name })
    newGroupName.value = ''
    ElMessage.success('分组已创建')
  } catch (err) {
    ElMessage.error(readError(err, '创建分组失败'))
  }
}

async function renameGroup(category) {
  try {
    const { value } = await ElMessageBox.prompt('输入新的分组名称', '重命名分组', {
      inputValue: category.name,
      inputValidator: value => !!value?.trim() || '分组名称不能为空',
    })
    const name = value.trim()
    if (!name || name === category.name) return
    await bookshelf.renameCategory(category.id, { name })
    ElMessage.success('分组已重命名')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名失败'))
  }
}

async function deleteGroup(category) {
  try {
    await ElMessageBox.confirm(`确定删除分组“${category.name}”吗？`, '删除分组', { type: 'warning' })
    await bookshelf.removeCategory(category.id)
    ElMessage.success('分组已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除分组失败'))
  }
}

async function moveGroup(category, direction) {
  const categories = [...bookshelf.categories]
  const index = categories.findIndex(item => item.id === category.id)
  const targetIndex = index + direction
  if (index < 0 || targetIndex < 0 || targetIndex >= categories.length) return
  const [moved] = categories.splice(index, 1)
  categories.splice(targetIndex, 0, moved)
  try {
    await bookshelf.reorderCategoryIds(categories.map(item => item.id))
    ElMessage.success('分组排序已更新')
  } catch (err) {
    ElMessage.error(readError(err, '分组排序失败'))
  }
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.overlay-actions,
.manage-footer {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.overlay-actions {
  margin-top: 4px;
}

.import-form {
  display: grid;
  gap: 12px;
}

.upload-icon {
  color: var(--app-primary);
  font-size: 32px;
}

.upload-text {
  color: var(--app-text-muted);
}

.group-list {
  display: grid;
  gap: 10px;
}

.group-row,
.group-create {
  display: grid;
  align-items: center;
  gap: 10px;
}

.manage-table {
  margin-bottom: 12px;
}

.mobile-manage-list {
  display: none;
}

.mobile-manage-card {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-manage-card header,
.mobile-manage-card footer {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-manage-card header button {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 3px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
  text-align: left;
}

.mobile-manage-card strong,
.mobile-manage-card span,
.mobile-manage-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-manage-card strong {
  font-size: 14px;
}

.mobile-manage-card span,
.mobile-manage-card p {
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-manage-card p {
  margin: 0;
}

.mobile-manage-card footer {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.text-button {
  padding: 0;
}

.manage-footer {
  align-items: center;
  padding-top: 10px;
  border-top: 1px solid var(--app-border);
}

.check-tip {
  color: var(--app-text-muted);
  font-size: 13px;
}

.group-create {
  grid-template-columns: minmax(0, 1fr) auto;
  margin-bottom: 12px;
}

.group-row {
  grid-template-columns: minmax(0, 1fr) auto;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.group-set-table {
  margin-bottom: 12px;
}

.group-set-footer {
  margin-top: 12px;
}

.radio-cell {
  display: inline-flex;
  width: 14px;
  height: 14px;
  border: 1px solid var(--app-border);
  border-radius: 50%;
}

.radio-cell.active {
  border-color: var(--el-color-primary);
  box-shadow: inset 0 0 0 4px #fff;
  background: var(--el-color-primary);
}

.group-actions {
  display: inline-flex;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.bookmark-editor {
  display: grid;
  gap: 10px;
}

.file-overlay {
  display: grid;
  gap: 12px;
}

.file-overlay-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.file-overlay-head > div:first-child {
  display: grid;
  gap: 2px;
}

.file-overlay-head span,
.mobile-file-card p,
.result-row small {
  color: var(--app-text-muted);
  font-size: 12px;
}

.file-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.file-breadcrumb button,
.file-name,
.mobile-file-name {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: 6px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.file-name span,
.mobile-file-name span,
.mobile-file-card p,
.result-row span,
.result-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-table {
  width: 100%;
}

.mobile-file-list {
  display: none;
}

.mobile-file-card {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-file-card header,
.mobile-file-card footer {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-file-card header {
  justify-content: space-between;
}

.mobile-file-card p {
  margin: 0;
}

.mobile-file-card footer {
  flex-wrap: wrap;
}

.backup-overlay {
  display: grid;
  gap: 12px;
}

.mobile-backup-list {
  display: none;
}

.mobile-backup-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-backup-card div {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.mobile-backup-card strong,
.mobile-backup-card span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-backup-card span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.result-list {
  display: grid;
  gap: 8px;
}

.result-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.user-overlay {
  display: grid;
  gap: 12px;
}

.permission-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.mobile-user-list {
  display: none;
}

.mobile-user-card {
  display: grid;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-user-card header {
  display: grid;
  gap: 2px;
}

.mobile-user-card span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.replace-overlay {
  display: grid;
  gap: 12px;
}

.mobile-rule-list {
  display: none;
}

.mobile-rule-card {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-rule-card header,
.mobile-rule-card footer,
.replace-test-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-rule-card header {
  justify-content: space-between;
}

.mobile-rule-card header > div {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.mobile-rule-card strong,
.mobile-rule-card span,
.mobile-rule-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-rule-card span,
.mobile-rule-card p,
.msg-muted {
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-rule-card p {
  margin: 0;
}

.replace-test-actions {
  margin-bottom: 8px;
}

.msg-success {
  color: var(--el-color-success);
  font-size: 12px;
}

.replace-test-output {
  max-height: 180px;
  overflow: auto;
  margin: 0;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  background: rgba(255, 255, 255, 0.68);
  color: var(--app-text);
  white-space: pre-wrap;
}

.rss-overlay-grid {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  gap: 14px;
  min-height: calc(100vh - 150px);
}

.rss-overlay-panel {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  min-width: 0;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  background: rgba(255, 255, 255, 0.62);
}

.rss-overlay-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 12px;
  border-bottom: 1px solid var(--app-border);
}

.rss-overlay-head > div:first-child {
  display: grid;
  gap: 2px;
}

.rss-overlay-head span,
.rss-source-row small,
.rss-article-row small,
.rss-article-row span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.rss-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.rss-source-list,
.rss-article-list {
  display: grid;
  align-content: start;
  max-height: calc(100vh - 230px);
  overflow: auto;
}

.rss-source-row,
.rss-article-row {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--app-border);
}

.rss-source-row {
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
}

.rss-source-row.active {
  background: rgba(145, 118, 62, 0.12);
}

.rss-source-row button,
.rss-article-row button {
  display: grid;
  min-width: 0;
  gap: 3px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
  text-align: left;
}

.rss-source-row strong,
.rss-source-row small,
.rss-article-row strong,
.rss-article-row small,
.rss-article-row span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rss-source-tools {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.rss-article-row {
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
}

.rss-article-row.read {
  opacity: 0.68;
}

.rss-reader {
  display: grid;
  gap: 12px;
}

.rss-reader h2 {
  margin: 0;
  font-size: 22px;
}

.rss-reader small {
  color: var(--app-text-muted);
}

.rss-reader p {
  margin: 0;
  color: var(--app-text);
  font-size: 17px;
  line-height: 1.9;
  white-space: pre-wrap;
}

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
  .desktop-manage-table {
    display: none;
  }

  .mobile-manage-list {
    display: grid;
    max-height: calc(100vh - 220px);
    overflow: auto;
    gap: 10px;
    margin-bottom: 12px;
  }

  .manage-footer {
    align-items: stretch;
    display: grid;
  }

  .overlay-actions {
    display: grid;
  }

  .file-overlay-head {
    align-items: flex-start;
    display: grid;
  }

  .file-actions {
    justify-content: flex-start;
  }

  .desktop-file-table {
    display: none;
  }

  .desktop-user-table {
    display: none;
  }

  .desktop-replace-table {
    display: none;
  }

  .desktop-backup-table {
    display: none;
  }

  .mobile-file-list {
    display: grid;
    max-height: 48vh;
    overflow: auto;
    gap: 10px;
  }

  .result-row {
    grid-template-columns: 1fr;
  }

  .mobile-user-list {
    display: grid;
    gap: 10px;
  }

  .mobile-rule-list {
    display: grid;
    gap: 10px;
  }

  .mobile-backup-list {
    display: grid;
    gap: 10px;
  }

  .rss-overlay-grid {
    grid-template-columns: 1fr;
    min-height: 0;
  }

  .rss-source-list,
  .rss-article-list {
    max-height: 40vh;
  }

  .rss-overlay-head {
    align-items: flex-start;
  }

  .rss-source-row,
  .rss-article-row {
    grid-template-columns: 1fr;
  }

  .rss-source-tools {
    flex-wrap: wrap;
    justify-content: flex-start;
  }
}

</style>
