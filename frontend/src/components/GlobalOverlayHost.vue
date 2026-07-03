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
    :cover-editable="bookInfoInShelf"
    :cover-uploading="coverUploadingBookId === overlay.bookInfoBook?.id"
    :show-update-switch="bookInfoInShelf && Number(overlay.bookInfoBook?.sourceId || 0) > 0"
    :can-update="overlay.bookInfoBook?.canUpdate !== false"
    :update-switch-loading="updatingBookId === overlay.bookInfoBook?.id"
    :browser-cache-count="bookInfoBrowserCacheCount"
    :in-shelf="bookInfoInShelf"
    :show-category-action="bookInfoInShelf"
    :show-local-refresh-action="bookInfoInShelf && Number(overlay.bookInfoBook?.sourceId || 0) <= 0"
    :local-refresh-loading="refreshingBookId === overlay.bookInfoBook?.id"
    @cover-upload="uploadBookInfoCover"
    @can-update-change="toggleBookCanUpdate"
    @category-action="setBookGroup(overlay.bookInfoBook)"
    @local-refresh="refreshLocalBookInfo(overlay.bookInfoBook)"
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
  </BookInfoDialog>

  <BookEditDialog
    v-model="overlay.bookEditVisible"
    :book="overlay.bookEditBook"
    :saving="editingBookSaving"
    @save="saveEditedBook"
  />

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
      <el-select v-model="importDraft.categoryIds" placeholder="分组（可多选）" multiple clearable>
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
      </el-select>
      <el-select
        v-if="importIsText"
        v-model="importDraft.tocRule"
        filterable
        allow-create
        clearable
        default-first-option
        :loading="tocRulesLoading"
        placeholder="目录规则（可选，留空自动识别）"
      >
        <el-option v-for="rule in tocRuleOptions" :key="rule.id" :label="rule.name" :value="rule.rule">
          <div class="toc-rule-option">
            <strong>{{ rule.name }}</strong>
            <span>{{ rule.rule }}</span>
          </div>
        </el-option>
      </el-select>
      <el-input
        v-if="importIsText"
        v-model="importDraft.tocRule"
        type="textarea"
        :rows="2"
        placeholder="TXT目录规则（可选，留空使用默认规则，例如：^第.+章.*$）"
      />
      <el-select v-if="importIsEPUB" v-model="importDraft.tocRule" placeholder="EPUB 目录规则">
        <el-option v-for="rule in epubTocRuleOptions" :key="rule.value" :label="rule.label" :value="rule.value" />
      </el-select>
      <div v-if="importDraft.file" class="direct-import-preview">
        <div>
          <strong>{{ importPreview ? `已解析 ${importPreview.chapterCount || 0} 章` : '尚未解析目录' }}</strong>
          <el-button size="small" text :loading="previewingImport" @click="previewImportFile">重新解析</el-button>
        </div>
        <div v-if="importPreview?.chapters?.length" class="direct-import-chapters">
          <span v-for="chapter in importPreview.chapters" :key="chapter.index">{{ chapter.title }}</span>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="overlay.importBookVisible = false">取消</el-button>
      <el-button type="primary" :loading="importingBook" :disabled="!importDraft.file || !importPreview" @click="importLocalBook">导入</el-button>
    </template>
  </el-dialog>

  <el-drawer
    v-model="overlay.bookManageVisible"
    title="书架管理"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-manage-drawer"
  >
    <div class="manage-head">
      <el-input v-model="manageKeyword" placeholder="搜索书名、作者或文件名" clearable size="small" />
      <div class="manage-head-actions">
        <el-button size="small" text @click="selectAllManagedBooks">全选</el-button>
        <el-button size="small" text @click="clearManagedSelection">清空</el-button>
      </div>
    </div>
    <el-table
      :data="filteredManagedBooks"
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
        <template #default="{ row }">{{ categoryName(row) }}</template>
      </el-table-column>
      <el-table-column label="章节" min-width="150">
        <template #default="{ row }">
          <span>共 {{ row.chapterCount || 0 }} 章</span><br>
          <span>阅读进度：{{ progressLabel(row) }}</span>
          <template v-if="Number(row.sourceId || 0) > 0">
            <br><span>服务器缓存：{{ serverCacheCount(row) }} 章</span>
          </template>
          <br><span>浏览器缓存：{{ localCacheCount(row) }} 章</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button text class="text-button" @click="overlay.openBookEdit(row)">编辑</el-button>
          <el-button text class="text-button" @click="setBookGroup(row)">分组</el-button>
          <el-dropdown @command="cacheBook(row, $event)">
            <el-button text class="text-button" :loading="cachingBookId === row.id">
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBookLocal">缓存到浏览器</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) > 0" command="cacheBook">缓存到服务器</el-dropdown-item>
                <el-dropdown-item command="deleteBookLocalCache">删除浏览器缓存</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) > 0" command="deleteBookCache">删除服务器缓存</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) === 0" disabled>本地书无需服务器缓存</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown @command="exportBook(row, $event)">
            <el-button text class="text-button">
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
      </el-table-column>
    </el-table>
    <div v-if="filteredManagedBooks.length" class="mobile-manage-list">
      <article v-for="book in filteredManagedBooks" :key="book.id" class="mobile-manage-card" :class="{ selected: selectedBookIds.includes(book.id) }">
        <header>
          <el-checkbox :model-value="selectedBookIds.includes(book.id)" @change="value => toggleManagedBook(book.id, value)" />
          <span
            class="mobile-manage-cover"
            :class="{ 'has-cover': hasBookCover(book) }"
            :style="coverStyle(book)"
          >{{ coverInitial(book) }}</span>
          <button type="button" @click="overlay.openBookInfo(book)">
            <strong>{{ book.title }}</strong>
            <span>{{ book.author || '未知作者' }} · {{ categoryName(book) }}</span>
            <span>{{ Number(book.sourceId || 0) > 0 ? '远程书籍' : '本地书籍' }} · {{ progressLabel(book) }}</span>
          </button>
        </header>
        <p>共 {{ book.chapterCount || 0 }} 章<template v-if="Number(book.sourceId || 0) > 0"> · 服务器缓存 {{ serverCacheCount(book) }} 章</template> · 浏览器缓存 {{ localCacheCount(book) }} 章<template v-if="book.lastChapter"> · 最新：{{ book.lastChapter }}</template></p>
        <footer>
          <el-button size="small" text @click="overlay.openBookEdit(book)">编辑</el-button>
          <el-button size="small" text @click="setBookGroup(book)">分组</el-button>
          <el-dropdown @command="cacheBook(book, $event)">
            <el-button size="small" text :loading="cachingBookId === book.id">
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBookLocal">缓存到浏览器</el-dropdown-item>
                <el-dropdown-item v-if="Number(book.sourceId || 0) > 0" command="cacheBook">缓存到服务器</el-dropdown-item>
                <el-dropdown-item command="deleteBookLocalCache">删除浏览器缓存</el-dropdown-item>
                <el-dropdown-item v-if="Number(book.sourceId || 0) > 0" command="deleteBookCache">删除服务器缓存</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown @command="exportBook(book, $event)">
            <el-button size="small" text>
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
        </footer>
      </article>
    </div>
    <el-empty v-else class="mobile-manage-empty" description="没有匹配的书籍" />
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
      <el-dropdown @command="handleBatchMoreCommand">
        <el-button :disabled="!selectedBookIds.length" :loading="batchBusy">
          更多批量操作<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="cache">批量缓存到服务器</el-dropdown-item>
            <el-dropdown-item command="clear-cache">批量清服务器缓存</el-dropdown-item>
            <el-dropdown-item command="export">批量导出</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-button @click="overlay.bookManageVisible = false">取消</el-button>
    </div>
  </el-drawer>

  <el-drawer
    v-model="overlay.bookGroupVisible"
    :title="overlay.bookGroupMode === 'set' ? '设置分组' : '分组管理'"
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
    @opened="handleBookGroupOpened"
    @closed="destroyGroupSortable"
  >
    <template v-if="overlay.bookGroupMode === 'set'">
      <el-table :data="groupSetRows" row-key="id" class="group-set-table" @row-click="toggleBookGroupSelection">
        <el-table-column width="46">
          <template #default="{ row }">
            <el-checkbox :model-value="isBookGroupSelected(row)" @change="() => toggleBookGroupSelection(row)" @click.stop />
          </template>
        </el-table-column>
        <el-table-column label="分组名">
          <template #default="{ row }">
            <span class="group-set-name">
              <span>{{ row.name }}</span>
              <small>{{ row.description }}</small>
            </span>
          </template>
        </el-table-column>
      </el-table>
      <div class="manage-footer group-set-footer">
        <el-button type="primary" :loading="settingCategorySaving" @click="saveBookGroupSetting">确认</el-button>
        <el-button @click="overlay.bookGroupVisible = false">取消</el-button>
      </div>
    </template>
    <template v-else>
      <el-table ref="groupManageTableRef" :data="groupManageRows" row-key="id" class="group-manage-table">
        <el-table-column width="46">
          <template #default>
            <button
              type="button"
              class="group-drag-handle"
              title="拖动排序"
            >
              <el-icon><Rank /></el-icon>
            </button>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="分组名" min-width="130">
          <template #default="{ row }">
            <span class="group-table-name">
              <span>{{ row.name }}</span>
              <small>{{ groupBookCount(row) }} 本</small>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="显示" width="120">
          <template #default="{ row }">
            <el-switch
              :model-value="row.show !== false"
              :loading="visibilitySavingId === row.id"
              active-text="显示"
              inactive-text="隐藏"
              @change="value => toggleGroupVisibility(row, value)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" min-width="180">
          <template #default="{ row }">
            <el-button size="small" text @click="renameGroup(row)">编辑</el-button>
            <el-button
              v-if="groupBookCount(row) === 0"
              size="small"
              text
              type="danger"
              @click="deleteGroup(row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!bookshelf.categories.length" description="还没有自定义分组" />
      <div class="manage-footer group-manage-footer">
        <el-button type="primary" @click="createCategory">添加分组</el-button>
        <el-button v-if="isGroupOrderDirty" type="primary" :loading="groupOrderSaving" @click="saveGroupOrderDraft">保存排序</el-button>
        <el-button @click="overlay.bookGroupVisible = false">取消</el-button>
      </div>
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
      @load-all="searchAllCurrentBookContent"
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
        @remove-many="removeBookmarkItems"
        @import="importBookmarkItems"
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
    v-model="overlay.localStoreVisible"
    title="本地书仓"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-local-store-drawer"
    destroy-on-close
  >
    <LocalStore embedded />
  </el-drawer>

  <el-drawer
    v-model="overlay.webdavVisible"
    title="WebDAV"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-file-drawer"
  >
    <WebDAVBrowser :is-mobile="isMobileOverlay" />
  </el-drawer>

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
          <span>保存当前数据到 WebDAV，或从备份包恢复</span>
        </div>
        <div class="file-actions">
          <el-button size="small" type="primary" :icon="Upload" :loading="backupLoading" @click="runBackup">保存到 WebDAV</el-button>
          <el-upload :show-file-list="false" :auto-upload="false" accept=".zip" @change="restoreBackup">
            <el-button size="small" :icon="Refresh" :loading="restoreLoading">恢复备份包</el-button>
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
          <el-button size="small" type="primary" :icon="Edit" @click="openCreateUserDialog">新增</el-button>
          <el-button size="small" :icon="Refresh" :loading="usersLoading" @click="loadUsers">刷新</el-button>
          <el-button size="small" :icon="Delete" :loading="cleanupLoading" @click="cleanupInactive">清理不活跃用户</el-button>
        </div>
      </header>
      <el-table :data="users" stripe v-loading="usersLoading" class="desktop-user-table" @selection-change="onUserSelectionChange">
        <el-table-column type="selection" width="44" :selectable="isUserDeletable" />
        <el-table-column prop="username" label="用户名" min-width="140" />
        <el-table-column prop="role" label="角色" width="90" />
        <el-table-column prop="bookCount" label="书籍" width="80" />
        <el-table-column prop="sourceCount" label="全局书源" width="100" />
        <el-table-column label="权限" min-width="300">
          <template #default="{ row }">
            <div class="permission-row">
              <el-switch v-model="row.canEditSources" size="small" active-text="书源" @change="updateUserPermission(row)" />
              <el-switch v-model="row.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(row)" />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="{ row }">
            <el-button text @click="resetPassword(row)">重置密码</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="users.length" v-loading="usersLoading" class="mobile-user-list">
        <article v-for="user in users" :key="user.id" class="mobile-user-card">
          <header>
            <el-checkbox :disabled="!isUserDeletable(user)" :model-value="selectedUserIds.includes(user.id)" @change="toggleUserSelection(user.id, $event)" />
            <div>
              <strong>{{ user.username }}</strong>
              <span>{{ user.role }} · 书籍 {{ user.bookCount || 0 }} · 全局书源 {{ user.sourceCount || 0 }}</span>
            </div>
          </header>
          <div class="permission-row">
            <el-switch v-model="user.canEditSources" size="small" active-text="书源" @change="updateUserPermission(user)" />
            <el-switch v-model="user.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(user)" />
            <el-button size="small" text @click="resetPassword(user)">重置密码</el-button>
          </div>
        </article>
      </div>
      <footer v-if="users.length" class="user-manage-footer">
        <span class="check-tip">已选择 {{ selectedUserIds.length }} 个</span>
        <el-button size="small" type="danger" :disabled="!selectedUserIds.length" :loading="deletingUsers" @click="deleteSelectedUsers">批量删除</el-button>
      </footer>
      <el-empty v-if="!usersLoading && !users.length" description="暂无用户，或当前账号无管理员权限" />
    </section>
  </el-drawer>

  <el-dialog v-model="userCreateDialog" title="新增用户" width="420px" :fullscreen="isMobileOverlay">
    <el-form label-position="top">
      <el-form-item label="用户名"><el-input v-model="userDraft.username" autocomplete="on" /></el-form-item>
      <el-form-item label="密码"><el-input v-model="userDraft.password" type="password" show-password autocomplete="new-password" /></el-form-item>
      <el-form-item label="角色">
        <el-select v-model="userDraft.role">
          <el-option label="普通用户" value="user" />
          <el-option label="管理员" value="admin" />
        </el-select>
      </el-form-item>
      <el-form-item label="权限">
        <div class="permission-row">
          <el-switch v-model="userDraft.canEditSources" active-text="书源" />
          <el-switch v-model="userDraft.canAccessStore" active-text="书仓" />
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="userCreateDialog = false">取消</el-button>
      <el-button type="primary" :loading="creatingUser" @click="createManagedUser">保存</el-button>
    </template>
  </el-dialog>

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
          <el-button size="small" :icon="Upload" :loading="replaceRuleImporting" @click="triggerReplaceRuleImport">导入</el-button>
          <el-button size="small" type="danger" plain :icon="Delete" :disabled="!selectedReplaceRuleIds.length" @click="deleteSelectedReplaceRules">批量删除</el-button>
          <el-button size="small" :icon="Refresh" :loading="replaceRulesLoading" @click="loadReplaceRules">刷新</el-button>
          <input ref="replaceRuleFileInput" class="visually-hidden-file" type="file" accept=".json,application/json" @change="importReplaceRuleFile" />
        </div>
      </header>
      <el-table :data="replaceRules" stripe v-loading="replaceRulesLoading" class="desktop-replace-table" @selection-change="onReplaceRuleSelectionChange">
        <el-table-column type="selection" width="44" />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="scope" label="替换范围" min-width="150" show-overflow-tooltip />
        <el-table-column prop="pattern" label="匹配" min-width="180" show-overflow-tooltip />
        <el-table-column prop="replacement" label="替换为" min-width="160" show-overflow-tooltip />
        <el-table-column label="正则" width="80">
          <template #default="{ row }">
            {{ normalizeReplaceRule(row).isRegex ? '是' : '否' }}
          </template>
        </el-table-column>
        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <el-switch :model-value="normalizeReplaceRule(row).enabled" size="small" @change="value => toggleReplaceRule(row, value)" />
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
            <el-checkbox :model-value="selectedReplaceRuleIds.includes(rule.id)" @change="toggleReplaceRuleSelection(rule.id, $event)" />
            <div>
              <strong>{{ rule.name || '未命名规则' }}</strong>
              <em>{{ normalizeReplaceRule(rule).scope }}</em>
              <span>{{ rule.pattern }}</span>
            </div>
            <el-switch :model-value="normalizeReplaceRule(rule).enabled" size="small" @change="value => toggleReplaceRule(rule, value)" />
          </header>
          <p>替换为：{{ rule.replacement || '空' }}</p>
          <p>模式：{{ normalizeReplaceRule(rule).isRegex ? '正则表达式' : '普通文本' }}</p>
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
      <el-form-item label="替换范围"><el-input v-model="replaceRuleDraft.scope" placeholder="* 或 书名 或 书名;书籍地址" /></el-form-item>
      <el-form-item><el-switch v-model="replaceRuleDraft.isRegex" active-text="使用正则表达式" inactive-text="普通文本" /></el-form-item>
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
  >
    <RSSManager :is-mobile="isMobileOverlay" />
  </el-drawer>
</template>

<script setup>
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Sortable from 'sortablejs'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Delete, Edit, Rank, Refresh, Upload, UploadFilled } from '@element-plus/icons-vue'
import * as adminApi from '../api/admin'
import { cacheBookContent, listChapters, listTXTTocRules, previewLocalBook, refreshLocalBook, updateBook, updateBookCategory } from '../api/books'
import * as backupApi from '../api/backup'
import * as replaceRulesApi from '../api/replaceRules'
import { listSources } from '../api/sources'
import { uploadAsset } from '../api/uploads'
import { bookHasCategory, mergeShelfBook, useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { useUserStore } from '../stores/user'
import { useBookBookmarks } from '../composables/useBookBookmarks'
import { useBookContentSearch } from '../composables/useBookContentSearch'
import { useOverlayUserManagement } from '../composables/useOverlayUserManagement'
import { useOverlayReplaceRules } from '../composables/useOverlayReplaceRules'
import { useOverlayBackups } from '../composables/useOverlayBackups'
import { useOverlayBookmarkActions } from '../composables/useOverlayBookmarkActions'
import { useOverlayBookImport } from '../composables/useOverlayBookImport'
import { useOverlayBookGroups } from '../composables/useOverlayBookGroups'
import { useOverlayBookInfo } from '../composables/useOverlayBookInfo'
import { bookCoverUrl, hasBookCover } from '../utils/bookCover'
import { cacheBookChaptersToBrowser, clearBookBrowserChapterCache, countBooksBrowserCachedChapters, listBookBrowserCachedChapters } from '../utils/bookChapterCache'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { createBookCategoryNameResolver } from '../utils/bookCategory'
import { localBookSearchText, normalizeLocalBookSearch } from '../utils/localBook'
import { epubTocRuleOptions } from '../utils/localBookToc'
import { invalidateReaderDataCache, writeReaderDataCache } from '../utils/readerDataCache'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { applyRestoreResult } from '../utils/restoreSync'
import BookEditDialog from './BookEditDialog.vue'
import BookInfoDialog from './BookInfoDialog.vue'
import RSSManager from './RSSManager.vue'
import WebDAVBrowser from './WebDAVBrowser.vue'
import ReaderBookmarkPanel from './reader/ReaderBookmarkPanel.vue'
import ReaderSearchPanel from './reader/ReaderSearchPanel.vue'

const LocalStore = defineAsyncComponent(() => import('../views/LocalStore.vue'))

const router = useRouter()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const userStore = useUserStore()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)

const selectedBookIds = ref([])
const batchBusy = ref(false)
const cachingBookId = ref(null)
const {
  importing: importingBook,
  previewing: previewingImport,
  previewData: importPreview,
  draft: importDraft,
  tocRuleOptions,
  tocRulesLoading,
  isText: importIsText,
  isEPUB: importIsEPUB,
  supportsTocRule: importSupportsTocRule,
  open: loadImportCategories,
  pickFile: pickImportFile,
  preview: previewImportFile,
  importBook: importLocalBook,
} = useOverlayBookImport({
  visible: computed(() => overlay.importBookVisible),
  loadCategories: () => warmOverlayCategories(),
  listTocRules: () => listTXTTocRules(),
  previewBook: (...args) => previewLocalBook(...args),
  importBook: payload => bookshelf.importTXT(payload),
  close: () => {
    overlay.importBookVisible = false
  },
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const sourceRows = ref([])
const contentSearchBook = computed(() => overlay.searchBook)
const contentSearchBookId = computed(() => overlay.searchBook?.id)
const {
  keyword: contentKeyword,
  results: contentResults,
  loading: contentSearching,
  searched: contentSearched,
  hasMore: contentHasMore,
  status: contentSearchStatus,
  reset: resetCurrentBookContentSearch,
  search: searchCurrentBookContent,
  loadMore: loadMoreCurrentBookContent,
  loadAll: searchAllCurrentBookContent,
} = useBookContentSearch({
  bookId: contentSearchBookId,
  book: contentSearchBook,
  chapters: [],
  onError: error => ElMessage.error(readError(error, '搜索正文失败')),
})
const contentSearchBookKey = ref('')
const bookmarkBookId = computed(() => overlay.bookmarkBook?.id)
const {
  items: bookmarkItems,
  loading: bookmarkLoading,
  mutating: bookmarkSaving,
  load: loadBookmarkItems,
  reset: resetBookmarkItems,
  update: updateBookmarkData,
  remove: removeBookmarkData,
  removeMany: removeBookmarkRows,
  importPayloads: importBookmarkPayloads,
  handleUpdated: handleBookmarksUpdated,
} = useBookBookmarks({
  bookId: bookmarkBookId,
  isActive: () => overlay.bookmarkVisible,
  onLoadError: error => ElMessage.error(readError(error, '加载书签失败')),
})
const {
  editorVisible: bookmarkEditorVisible,
  draft: bookmarkDraft,
  jump: jumpToBookmark,
  openEditor: openBookmarkEditor,
  saveEdit: saveBookmarkEdit,
  removeOne: removeBookmarkItem,
  removeMany: removeBookmarkItems,
  importRows: importBookmarkItems,
} = useOverlayBookmarkActions({
  getBook: () => overlay.bookmarkBook,
  closePanel: () => {
    overlay.bookmarkVisible = false
  },
  navigate: routeLocation => router.push(routeLocation),
  update: updateBookmarkData,
  remove: removeBookmarkData,
  removeMany: removeBookmarkRows,
  importPayloads: importBookmarkPayloads,
  confirm: (...args) => ElMessageBox.confirm(...args),
  onSuccess: message => ElMessage.success(message),
  onInvalidImport: message => ElMessage.error(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  backups,
  backupLoading,
  listLoading: backupListLoading,
  restoreLoading,
  load: loadBackups,
  run: runBackup,
  download: downloadBackupFile,
  restore: restoreBackup,
} = useOverlayBackups({
  ...backupApi,
  restoreBackup: backupApi.restoreLegadoBackup,
  applyRestoreResult,
  saveBlob: downloadBlob,
  createFormData: () => new FormData(),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  users,
  usersLoading,
  cleanupLoading,
  deletingUsers,
  creatingUser,
  createDialogVisible: userCreateDialog,
  selectedUserIds,
  draft: userDraft,
  load: loadUsers,
  handleUpdated: handleUsersUpdated,
  clearRefresh: clearUsersRefreshTimer,
  isDeletable: isUserDeletable,
  changeSelection: onUserSelectionChange,
  toggleSelection: toggleUserSelection,
  openCreateDialog: openCreateUserDialog,
  create: createManagedUser,
  resetPassword,
  removeSelected: deleteSelectedUsers,
  updatePermission: updateUserPermission,
  cleanupInactive,
} = useOverlayUserManagement({
  userStore,
  getCurrentUserId: () => userStore.profile?.id || null,
  isActive: () => overlay.userManageVisible,
  ...adminApi,
  prompt: (...args) => ElMessageBox.prompt(...args),
  confirm: (...args) => ElMessageBox.confirm(...args),
  onSuccess: message => ElMessage.success(message),
  onWarning: message => ElMessage.warning(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  rules: replaceRules,
  loading: replaceRulesLoading,
  importing: replaceRuleImporting,
  selectedIds: selectedReplaceRuleIds,
  fileInput: replaceRuleFileInput,
  dialogVisible: replaceRuleDialog,
  saving: replaceRuleSaving,
  testing: replaceRuleTesting,
  editingId: editingReplaceRuleId,
  draft: replaceRuleDraft,
  testText: replaceRuleTestText,
  testResult: replaceRuleTestResult,
  load: loadReplaceRules,
  handleUpdated: handleReplaceRulesUpdated,
  clearRefresh: clearReplaceRulesRefreshTimer,
  changeSelection: onReplaceRuleSelectionChange,
  toggleSelection: toggleReplaceRuleSelection,
  triggerImport: triggerReplaceRuleImport,
  importFile: importReplaceRuleFile,
  normalize: normalizeReplaceRule,
  openEditor: openReplaceRuleEditor,
  save: saveReplaceRule,
  toggle: toggleReplaceRule,
  runTest: runReplaceRuleTest,
  remove: removeReplaceRule,
  removeSelected: deleteSelectedReplaceRules,
} = useOverlayReplaceRules({
  isActive: () => overlay.replaceRulesVisible,
  ...replaceRulesApi,
  confirm: (...args) => ElMessageBox.confirm(...args),
  notifyUpdated: () => {
    window.dispatchEvent(new CustomEvent(
      'openreader:replace-rules-updated',
      { detail: { local: true } },
    ))
  },
  onSuccess: message => ElMessage.success(message),
  onWarning: message => ElMessage.warning(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const manageKeyword = ref('')
const windowWidth = ref(currentViewportWidth())
let sourceRowsRefreshTimer

const isMobileOverlay = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const wideDrawerDirection = computed(() => isMobileOverlay.value ? 'btt' : 'rtl')
const wideDrawerSize = computed(() => isMobileOverlay.value ? '88%' : '82%')
const narrowDrawerDirection = computed(() => isMobileOverlay.value ? 'btt' : 'rtl')
const narrowDrawerSize = computed(() => isMobileOverlay.value ? '86%' : '420px')
const bookInfoCategory = computed(() => overlay.bookInfoOptions.categoryName || categoryName(overlay.bookInfoBook))
const bookInfoSourceName = computed(() => {
  if (overlay.bookInfoOptions.sourceName) return overlay.bookInfoOptions.sourceName
  const sourceId = overlay.bookInfoBook?.sourceId
  if (!sourceId) return '本地'
  return sourceRows.value.find(source => Number(source.id) === Number(sourceId))?.name || '远程书籍'
})
const bookInfoProgress = computed(() => {
  const book = overlay.bookInfoBook
  return bookProgress(book)?.percent || 0
})
const bookInfoBrowserCacheCount = computed(() => (
  overlay.bookInfoBook?.id ? localCacheCount(overlay.bookInfoBook) : -1
))
const bookInfoInShelf = computed(() => isShelfBook(overlay.bookInfoBook))
const sourceStatusLabel = computed(() => overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍')
const managedBooks = computed(() => sortByShelfOrder(bookshelf.books, reader.progressByBook))
const filteredManagedBooks = computed(() => {
  const value = normalizeLocalBookSearch(manageKeyword.value)
  if (!value) return managedBooks.value
  return managedBooks.value.filter(book => manageBookSearchText(book).includes(value))
})
const {
  refreshingBookId,
  coverUploadingBookId,
  updatingBookId,
  editingBookSaving,
  refreshManagedBrowserCacheCounts,
  refreshBookInfoBrowserCacheCount,
  invalidateBookReaderCaches,
  refreshBookChaptersCache,
  mergedShelfBook,
  applyUpdatedBookToOverlay,
  localCacheCount,
  serverCacheCount,
  updateServerCacheCount,
  saveEditedBook,
  refreshLocalBookInfo,
  uploadBookInfoCover,
  toggleBookCanUpdate,
} = useOverlayBookInfo({
  overlay,
  bookshelf,
  getManagedBooks: () => managedBooks.value,
  countBrowserCachedChapters: countBooksBrowserCachedChapters,
  listBrowserCachedChapters: listBookBrowserCachedChapters,
  clearBrowserChapterCache: clearBookBrowserChapterCache,
  invalidateReaderData: invalidateReaderDataCache,
  listChapters,
  writeReaderData: writeReaderDataCache,
  refreshLocalBook,
  uploadAsset,
  updateBook,
  mergeBook: mergeShelfBook,
  emitBookInfoUpdated: book => {
    window.dispatchEvent(new CustomEvent('openreader:book-info-updated', {
      detail: { book },
    }))
  },
  emitReaderBookDataUpdated: detail => {
    window.dispatchEvent(new CustomEvent(
      'openreader:reader-book-data-updated',
      { detail },
    ))
  },
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  settingCategorySaving,
  visibilitySavingId,
  groupOrderSaving,
  groupManageTableRef,
  groupSetRows,
  groupManageRows,
  isGroupOrderDirty,
  groupBookCount,
  prepareOpen: prepareBookGroupOpen,
  isBookGroupSelected,
  toggleBookGroupSelection,
  saveBookGroupSetting,
  createCategory,
  renameGroup,
  toggleGroupVisibility,
  deleteGroup,
  handleBookGroupOpened,
  destroyGroupSortable,
  handleModeChange: handleBookGroupModeChange,
  saveGroupOrderDraft,
} = useOverlayBookGroups({
  overlay,
  bookshelf,
  getManagedBooks: () => managedBooks.value,
  updateBookCategory,
  categoryName,
  getBookProgress: bookProgress,
  emitBookInfoUpdated: data => {
    window.dispatchEvent(new CustomEvent('openreader:book-info-updated', {
      detail: { book: data },
    }))
  },
  prompt: (...args) => ElMessageBox.prompt(...args),
  confirm: (...args) => ElMessageBox.confirm(...args),
  createSortable: (...args) => Sortable.create(...args),
  nextFrame: nextTick,
  onSuccess: message => ElMessage.success(message),
  onWarning: message => ElMessage.warning(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

function manageBookSearchText(book) {
  return localBookSearchText(book, [
    progressLabel(book),
    categoryName(book),
  ])
}

function isShelfBook(book) {
  if (!book) return false
  if (book.id && bookshelf.books.some(item => Number(item.id) === Number(book.id))) return true
  const bookUrl = String(book.url || book.bookUrl || '').trim()
  if (!bookUrl) return false
  return bookshelf.books.some(item => String(item.url || item.bookUrl || '').trim() === bookUrl)
}
onMounted(() => {
  window.addEventListener('resize', updateWindowWidth, { passive: true })
  window.addEventListener('openreader:replace-rules-updated', handleReplaceRulesUpdated)
  window.addEventListener('openreader:bookmarks-updated', handleBookmarksUpdated)
  window.addEventListener('openreader:users-updated', handleUsersUpdated)
  window.addEventListener('openreader:sources-update', handleSourcesUpdated)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateWindowWidth)
  window.removeEventListener('openreader:replace-rules-updated', handleReplaceRulesUpdated)
  window.removeEventListener('openreader:bookmarks-updated', handleBookmarksUpdated)
  window.removeEventListener('openreader:users-updated', handleUsersUpdated)
  window.removeEventListener('openreader:sources-update', handleSourcesUpdated)
  clearReplaceRulesRefreshTimer()
  clearUsersRefreshTimer()
  clearSourceRowsRefreshTimer()
  destroyGroupSortable()
})

function updateWindowWidth() {
  windowWidth.value = currentViewportWidth()
}

watch(
  () => overlay.bookManageVisible || overlay.bookGroupVisible,
  async (visible) => {
    if (!visible) {
      if (!overlay.bookManageVisible) {
        manageKeyword.value = ''
        selectedBookIds.value = []
      }
      return
    }
    if (overlay.bookManageVisible) {
      const [categoryResult, booksResult] = await Promise.allSettled([
        warmOverlayCategories(),
        warmOverlayBooks(),
      ])
      if (booksResult.status === 'rejected') {
        ElMessage.error(readError(booksResult.reason, '加载书架数据失败'))
        return
      }
      if (categoryResult.status === 'rejected') {
        if (overlay.bookGroupVisible) {
          ElMessage.error(readError(categoryResult.reason, '加载分组失败'))
          return
        }
        ElMessage.warning(readError(categoryResult.reason, '分组加载失败，书架管理仍可使用'))
      }
      await refreshManagedBrowserCacheCounts()
    } else {
      try {
        await warmOverlayCategories()
      } catch (err) {
        ElMessage.error(readError(err, '加载分组失败'))
        return
      }
    }
    if (overlay.bookGroupVisible) prepareBookGroupOpen()
  },
)

watch(
  () => overlay.bookInfoVisible,
  async (visible) => {
    if (!visible) return
    const warmTasks = [warmOverlayCategories()]
    if (overlay.bookInfoBook?.id) warmTasks.push(warmOverlayBooks())
    const [categoryResult, booksResult] = await Promise.allSettled(warmTasks)
    if (categoryResult.status === 'rejected') {
      ElMessage.warning(readError(categoryResult.reason, '分组加载失败，书籍信息仍可查看'))
    }
    if (booksResult?.status === 'rejected') {
      ElMessage.warning(readError(booksResult.reason, '书架状态加载失败，书籍信息仍可查看'))
    }
    if (overlay.bookInfoBook?.sourceId && !sourceRows.value.length) {
      await loadSourceRows().catch((err) => {
        ElMessage.warning(readError(err, '书源加载失败，书籍信息仍可查看'))
      })
    }
    if (overlay.bookInfoBook?.id) {
      await refreshBookInfoBrowserCacheCount(overlay.bookInfoBook)
    }
  },
)

watch(
  () => overlay.bookGroupMode,
  mode => handleBookGroupModeChange(mode),
)

watch(
  () => overlay.searchBook?.id || overlay.searchBook?.bookUrl || '',
  (key) => {
    if (String(key || '') === contentSearchBookKey.value) return
    contentSearchBookKey.value = String(key || '')
    resetContentSearchState()
  },
)

async function warmOverlayCategories(options = {}) {
  return bookshelf.ensureCategoriesLoaded(options)
}

async function warmOverlayBooks(options = {}) {
  return bookshelf.ensureBooksLoaded({ all: true, ...options })
}

function resetContentSearchState() {
  contentKeyword.value = ''
  resetCurrentBookContentSearch()
}

watch(
  () => overlay.searchBookContentVisible,
  (visible) => {
    if (!visible) return
    const key = String(overlay.searchBook?.id || overlay.searchBook?.bookUrl || '')
    if (key && key !== contentSearchBookKey.value) {
      contentSearchBookKey.value = key
      resetContentSearchState()
    }
  },
)

watch(
  () => overlay.bookmarkVisible,
  async (visible) => {
    if (!visible) {
      resetBookmarkItems()
      return
    }
    await loadBookmarkItems()
  },
)

function progressLabel(book) {
  const progress = bookProgress(book)
  return `${Math.round((progress?.percent || 0) * 100)}%`
}

async function loadSourceRows() {
  const { data } = await listSources()
  sourceRows.value = data || []
}

function handleSourcesUpdated() {
  if (!shouldRefreshOverlaySources()) return
  scheduleSourceRowsRefresh()
}

function shouldRefreshOverlaySources() {
  return (overlay.bookInfoVisible && Number(overlay.bookInfoBook?.sourceId || 0) > 0) ||
    sourceRows.value.length > 0
}

function scheduleSourceRowsRefresh() {
  clearSourceRowsRefreshTimer()
  sourceRowsRefreshTimer = window.setTimeout(async () => {
    sourceRowsRefreshTimer = undefined
    try {
      await loadSourceRows()
    } catch {
      // Keep existing source names/groups; the next source action can recover.
    }
  }, 350)
}

function clearSourceRowsRefreshTimer() {
  if (!sourceRowsRefreshTimer) return
  window.clearTimeout(sourceRowsRefreshTimer)
  sourceRowsRefreshTimer = undefined
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

function selectAllManagedBooks() {
  selectedBookIds.value = filteredManagedBooks.value.map(book => book.id)
}

function clearManagedSelection() {
  selectedBookIds.value = []
}

function coverInitial(book) {
  if (hasBookCover(book)) return ''
  return (book?.title || '?').slice(0, 1)
}

function coverStyle(book) {
  const url = bookCoverUrl(book)
  return url ? { backgroundImage: `url(${url})` } : {}
}

function setBookGroup(book) {
  overlay.openBookGroup('set', book, {
    categoryName: categoryName(book),
    progress: bookProgress(book)?.percent || 0,
  })
}

async function batchAddCategory(category) {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    await bookshelf.batchSetCategory([...selectedBookIds.value], category.id, { action: 'category-add' })
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
    .filter(book => selectedBookIds.value.includes(book.id) && bookHasCategory(book, category.id))
    .map(book => book.id)
  if (!targetIds.length) {
    ElMessage.info('选中书籍不在该分组中')
    return
  }
  batchBusy.value = true
  try {
    await bookshelf.batchSetCategory(targetIds, category.id, { action: 'category-remove' })
    ElMessage.success(`已从“${category.name}”分组移除`)
  } catch (err) {
    ElMessage.error(readError(err, '批量移除分组失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchCacheBooks() {
  if (!selectedBookIds.value.length) return
  const remoteBookIds = selectedRemoteBookIds()
  if (!remoteBookIds.length) {
    ElMessage.info('选中的本地书无需服务器缓存')
    return
  }
  batchBusy.value = true
  try {
    const data = await bookshelf.batchCacheBooks(remoteBookIds)
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
    await bookshelf.loadBooks({ force: true, all: true })
  } catch (err) {
    ElMessage.error(readError(err, '批量缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

async function batchClearCache() {
  if (!selectedBookIds.value.length) return
  const remoteBookIds = selectedRemoteBookIds()
  if (!remoteBookIds.length) {
    ElMessage.info('选中的本地书没有服务器缓存')
    return
  }
  try {
    await ElMessageBox.confirm(`确定清理选中 ${remoteBookIds.length} 本远程书的章节缓存吗？`, '清理缓存', { type: 'warning' })
    batchBusy.value = true
    const data = await bookshelf.batchClearCache(remoteBookIds)
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
    for (const bookId of remoteBookIds) {
      const book = managedBooks.value.find(item => Number(item.id) === Number(bookId))
      if (book) updateServerCacheCount(book, 0)
    }
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    batchBusy.value = false
  }
}

function handleBatchMoreCommand(command) {
  if (command === 'cache') {
    batchCacheBooks()
  } else if (command === 'clear-cache') {
    batchClearCache()
  } else if (command === 'export') {
    batchExportBooks()
  }
}

function selectedRemoteBookIds() {
  const selected = new Set(selectedBookIds.value)
  return managedBooks.value
    .filter(book => selected.has(book.id) && Number(book.sourceId || 0) > 0)
    .map(book => book.id)
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

async function batchExportBooks() {
  if (!selectedBookIds.value.length) return
  batchBusy.value = true
  try {
    const bookIds = [...selectedBookIds.value]
    const blob = await bookshelf.exportSelectedBooks(bookIds, 'json')
    downloadBlob(blob, `openreader-books-${bookIds.length}.json`)
    ElMessage.success(`已导出 ${bookIds.length} 本书`)
  } catch (err) {
    ElMessage.error(readError(err, '批量导出失败'))
  } finally {
    batchBusy.value = false
  }
}

async function cacheBook(book, command) {
  if (Number(book?.sourceId || 0) === 0 && command !== 'cacheBookLocal' && command !== 'deleteBookLocalCache') {
    ElMessage.info('本地书无需服务器缓存')
    return
  }
  if (command === 'deleteBookCache') {
    await clearBookCache(book)
    return
  }
  if (command === 'deleteBookLocalCache') {
    await clearBookLocalCache(book)
    return
  }
  if (command === 'cacheBookLocal') {
    await cacheBookLocal(book)
    return
  }
  cachingBookId.value = book.id
  try {
    const chapterIndex = cacheStartChapterIndex(book)
    const { data } = await cacheBookContent(book.id, { all: true, count: 20, chapterIndex })
    if (data?.book) bookshelf.upsertBook(data.book)
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '缓存失败'))
  } finally {
    cachingBookId.value = null
  }
}

async function cacheBookLocal(book) {
  cachingBookId.value = book.id
  try {
    const { data } = await listChapters(book.id)
    const chapterIndex = cacheStartChapterIndex(book)
    const result = await cacheBookChaptersToBrowser(book, book.id, Array.isArray(data) ? data : [], {
      startIndex: chapterIndex,
      count: 100,
    })
    ElMessage.success(`已缓存到浏览器 ${result.cached}/${result.requested} 章`)
    await refreshManagedBrowserCacheCounts()
    await refreshBookInfoBrowserCacheCount(book)
  } catch (err) {
    ElMessage.error(readError(err, '缓存到浏览器失败'))
  } finally {
    cachingBookId.value = null
  }
}

function cacheStartChapterIndex(book) {
  const progress = bookProgress(book)
  const chapterIndex = Number(progress?.chapterIndex)
  return Number.isInteger(chapterIndex) && chapterIndex > 0 ? chapterIndex : 0
}

function bookProgress(book) {
  return newestBookProgress(book, reader.progressByBook)
}

async function clearBookCache(book) {
  cachingBookId.value = book.id
  try {
    const data = await bookshelf.batchClearCache([book.id])
    updateServerCacheCount(book, 0)
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    cachingBookId.value = null
  }
}

async function clearBookLocalCache(book) {
  cachingBookId.value = book.id
  try {
    const removed = await clearBookBrowserChapterCache(book, book.id)
    await refreshManagedBrowserCacheCounts()
    await refreshBookInfoBrowserCacheCount(book)
    ElMessage.success(`已清理浏览器缓存 ${removed} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '清理浏览器缓存失败'))
  } finally {
    cachingBookId.value = null
  }
}

async function exportBook(book, format = 'txt') {
  batchBusy.value = true
  try {
    const normalizedFormat = ['json', 'txt', 'epub'].includes(format) ? format : 'txt'
    const blob = await bookshelf.exportSelectedBooks([book.id], normalizedFormat)
    downloadBlob(blob, exportBookFilename(book, normalizedFormat))
    ElMessage.success(`已导出《${book.title}》`)
  } catch (err) {
    ElMessage.error(readError(err, '导出失败'))
  } finally {
    batchBusy.value = false
  }
}

function exportBookFilename(book, format) {
  const fallback = `book-${book?.id || Date.now()}`
  const title = String(book?.title || fallback).replace(/[\\/:*?"<>|]/g, '-').trim() || fallback
  return `${title}.${format === 'json' ? 'json' : format === 'epub' ? 'epub' : 'txt'}`
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
      match: Number.isInteger(result.resultCountWithinChapter) ? result.resultCountWithinChapter : undefined,
      percent: Number.isFinite(Number(result.percent)) ? Number(result.percent) : undefined,
      q: contentKeyword.value.trim() || undefined,
    },
  })
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

function joinPath(base, name) {
  return [base, name].filter(Boolean).join('/')
}

function buildSourceGroupOptions(rows) {
  const counts = new Map()
  for (const item of rows || []) {
    if (item?.enabled === false) continue
    const group = String(item?.group || '').trim()
    if (!group) continue
    counts.set(group, (counts.get(group) || 0) + 1)
  }
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([value, count]) => ({ value, label: value, count }))
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

.toc-rule-option {
  display: grid;
  gap: 2px;
  min-width: 0;
  line-height: 1.25;
}

.toc-rule-option strong,
.toc-rule-option span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.toc-rule-option span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.upload-icon {
  color: var(--app-primary);
  font-size: 32px;
}

.upload-text {
  color: var(--app-text-muted);
}

.direct-import-preview {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.direct-import-preview > div:first-child {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.direct-import-chapters {
  display: grid;
  max-height: 180px;
  overflow: auto;
  gap: 5px;
  padding: 8px;
  background: var(--app-bg-soft);
  color: var(--app-text-muted);
  font-size: 12px;
}

.manage-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.manage-head-actions {
  display: none;
  flex: 0 0 auto;
  gap: 6px;
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

.mobile-manage-card.selected {
  border-color: var(--app-primary);
  background: var(--app-primary-soft);
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

.mobile-manage-cover {
  display: grid;
  width: 34px;
  height: 46px;
  place-items: center;
  flex: 0 0 34px;
  color: #fffdf8;
  background: var(--app-primary);
  border-radius: 4px;
  font-size: 16px;
  font-weight: 800;
}

.mobile-manage-cover.has-cover {
  background-position: center;
  background-size: cover;
  color: transparent;
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

.mobile-manage-empty {
  display: none;
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

.group-manage-table {
  margin-bottom: 12px;
}

.group-drag-handle {
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--app-text-muted);
  cursor: move;
}

.group-drag-handle:hover {
  background: var(--app-bg-soft);
  color: var(--app-text);
}

.group-table-name {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.group-table-name span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-table-name small {
  color: var(--app-text-muted);
  font-size: 12px;
}

.group-set-table {
  margin-bottom: 12px;
}

.group-set-footer {
  margin-top: 12px;
}

.group-set-name {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.group-set-name span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-set-name small {
  color: var(--app-text-muted);
  font-size: 12px;
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

.file-overlay-head span {
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

.visually-hidden-file {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
  padding: 0;
  margin: -1px;
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
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-user-card header > div {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 2px;
}

.mobile-user-card span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.user-manage-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
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
  flex: 1;
  gap: 2px;
}

.mobile-rule-card strong,
.mobile-rule-card em,
.mobile-rule-card span,
.mobile-rule-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-rule-card em {
  color: var(--app-text-muted);
  font-size: 12px;
  font-style: normal;
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

@media (max-width: 750px) {
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

  .mobile-manage-empty {
    display: block;
  }

  .manage-footer {
    align-items: stretch;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .manage-footer :deep(.el-button),
  .manage-footer :deep(.el-dropdown),
  .manage-footer :deep(.el-dropdown .el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

  .manage-footer .check-tip {
    grid-column: 1 / -1;
    order: -1;
  }

  .group-set-footer {
    grid-template-columns: 1fr;
  }

  .manage-head {
    grid-template-columns: 1fr;
  }

  .manage-head-actions {
    display: flex;
    justify-content: flex-end;
  }

  .overlay-actions {
    display: grid;
  }

  .overlay-actions :deep(.el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

  .file-overlay-head {
    align-items: flex-start;
    display: grid;
  }

  .file-actions {
    justify-content: flex-start;
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

}

</style>
