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
    :browser-cache-count="bookInfoBrowserCacheCount"
    :show-category-action="!!overlay.bookInfoBook?.id"
    @cover-upload="uploadBookInfoCover"
    @can-update-change="toggleBookCanUpdate"
    @category-action="setBookGroup(overlay.bookInfoBook)"
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
      <el-button v-if="Number(overlay.bookInfoBook.sourceId || 0) > 0" plain :loading="refreshingBookId === overlay.bookInfoBook.id" @click="refreshBookInfo(overlay.bookInfoBook)">刷新目录</el-button>
      <el-button v-else plain :loading="refreshingBookId === overlay.bookInfoBook.id" @click="refreshLocalBookInfo(overlay.bookInfoBook)">刷新本地书</el-button>
      <el-button v-if="Number(overlay.bookInfoBook.sourceId || 0) > 0" plain :loading="sourceSwitchLoading" @click="openGlobalSourceSwitch(overlay.bookInfoBook)">换源</el-button>
      <el-button v-if="Number(overlay.bookInfoBook.sourceId || 0) > 0" plain :loading="cachingBookId === overlay.bookInfoBook.id" @click="cacheBook(overlay.bookInfoBook, 'cacheBookLocal')">缓存后续100章</el-button>
      <el-button v-if="Number(overlay.bookInfoBook.sourceId || 0) > 0" plain :loading="cachingBookId === overlay.bookInfoBook.id" @click="cacheBook(overlay.bookInfoBook, 'cacheBook')">缓存到服务器</el-button>
      <el-button v-if="Number(overlay.bookInfoBook.sourceId || 0) > 0" plain :loading="cachingBookId === overlay.bookInfoBook.id" @click="cacheBook(overlay.bookInfoBook, 'deleteBookLocalCache')">清浏览器缓存</el-button>
      <el-button v-if="Number(overlay.bookInfoBook.sourceId || 0) > 0" plain :loading="cachingBookId === overlay.bookInfoBook.id" @click="cacheBook(overlay.bookInfoBook, 'deleteBookCache')">清服务器缓存</el-button>
      <el-button plain @click="goDetail(overlay.bookInfoBook)">详情</el-button>
      <el-button plain :loading="loadingUpdates" @click="refreshShelf">刷新书架</el-button>
      <el-button plain type="danger" :loading="deletingBookId === overlay.bookInfoBook.id" @click="deleteBookFromInfo(overlay.bookInfoBook)">移出书架</el-button>
    </div>
  </BookInfoDialog>

  <el-drawer
    v-model="sourceSwitchVisible"
    title="切换书源"
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
    class="global-source-drawer"
    @open="loadGlobalSourceCandidates"
  >
    <SourceSwitchPanel
      :book="sourceSwitchBook"
      :sources="sourceSwitchCandidates"
      :loading="sourceSwitchLoading"
      :has-more="sourceSwitchHasMore"
      :changing-source="sourceSwitchChanging"
      :current-source-name="sourceSwitchCurrentName"
      :group="sourceSwitchGroup"
      :query="sourceSwitchQuery"
      :groups="sourceSwitchGroups"
      :stats="sourceSwitchStats"
      @refresh="refreshGlobalSourceCandidates"
      @load-more="loadMoreGlobalSourceCandidates"
      @group-change="changeGlobalSourceGroup"
      @query-change="changeGlobalSourceQuery"
      @show-info="reopenSourceSwitchBookInfo"
      @change="changeGlobalBookSource"
    />
  </el-drawer>

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
    <div class="manage-head">
      <el-input v-model="manageKeyword" placeholder="搜索书名或作者" clearable size="small" />
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
        <template #default="{ row }">{{ categoryName(row.categoryId) }}</template>
      </el-table-column>
      <el-table-column label="章节" min-width="150">
        <template #default="{ row }">
          <span>共 {{ row.chapterCount || 0 }} 章</span><br>
          <span>阅读进度：{{ progressLabel(row) }}</span>
          <template v-if="Number(row.sourceId || 0) > 0">
            <br><span>浏览器缓存：{{ localCacheCount(row) }} 章</span>
          </template>
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
                <el-dropdown-item v-if="Number(row.sourceId || 0) > 0" command="cacheBookLocal">缓存后续100章</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) > 0" command="cacheBook">缓存到服务器</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) > 0" command="deleteBookLocalCache">删除浏览器缓存</el-dropdown-item>
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
          <span class="mobile-manage-cover">{{ coverInitial(book) }}</span>
          <button type="button" @click="overlay.openBookInfo(book)">
            <strong>{{ book.title }}</strong>
            <span>{{ book.author || '未知作者' }} · {{ categoryName(book.categoryId) }}</span>
            <span>{{ Number(book.sourceId || 0) > 0 ? '远程书籍' : '本地书籍' }} · {{ progressLabel(book) }}</span>
          </button>
        </header>
        <p>共 {{ book.chapterCount || 0 }} 章<template v-if="Number(book.sourceId || 0) > 0"> · 浏览器缓存 {{ localCacheCount(book) }} 章</template><template v-if="book.lastChapter"> · 最新：{{ book.lastChapter }}</template></p>
        <footer>
          <el-button size="small" text @click="goDetail(book)">编辑</el-button>
          <el-button size="small" text @click="setBookGroup(book)">分组</el-button>
          <el-button v-if="Number(book.sourceId || 0) > 0" size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'cacheBookLocal')">缓存100章</el-button>
          <el-button v-if="Number(book.sourceId || 0) > 0" size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'deleteBookLocalCache')">清浏览器</el-button>
          <el-button v-if="Number(book.sourceId || 0) > 0" size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'cacheBook')">服务器缓存</el-button>
          <el-button v-if="Number(book.sourceId || 0) > 0" size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'deleteBookCache')">清服务器</el-button>
          <el-button size="small" text @click="exportBook(book)">导出</el-button>
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
            <el-dropdown-item command="cache">批量服务器缓存10章</el-dropdown-item>
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
          <span class="group-name">
            <span>{{ category.name }}</span>
            <small>{{ groupBookCount(category) }} 本</small>
          </span>
          <span class="group-actions">
            <el-button size="small" text @click="moveGroup(category, -1)">上移</el-button>
            <el-button size="small" text @click="moveGroup(category, 1)">下移</el-button>
            <el-button size="small" text @click="renameGroup(category)">重命名</el-button>
            <el-button
              v-if="groupBookCount(category) === 0"
              size="small"
              text
              type="danger"
              @click="deleteGroup(category)"
            >
              删除
            </el-button>
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
            <el-checkbox :model-value="selectedReplaceRuleIds.includes(rule.id)" @change="toggleReplaceRuleSelection(rule.id, $event)" />
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
  >
    <RSSManager :is-mobile="isMobileOverlay" />
  </el-drawer>
</template>

<script setup>
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Delete, Edit, Refresh, Upload, UploadFilled } from '@element-plus/icons-vue'
import { cleanupInactiveUsers, createUser, deleteUsers, listUsers, resetUserPassword, updateUser } from '../api/admin'
import { cacheBookContent, changeBookSource, checkBookUpdates, createBookmark, deleteBookmark, listBookSourceCandidates, listBookmarks, listChapters, refreshBook, refreshLocalBook, searchBookContent, updateBook, updateBookCategory, updateBookmark } from '../api/books'
import { downloadBackup, listBackups, restoreLegadoBackup, triggerBackup } from '../api/backup'
import { createReplaceRule, deleteReplaceRule, listReplaceRules, testReplaceRule, updateReplaceRule } from '../api/replaceRules'
import { listSources } from '../api/sources'
import { uploadAsset } from '../api/uploads'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { useUserStore } from '../stores/user'
import { cacheBookChaptersToBrowser, clearBookBrowserChapterCache, countBooksBrowserCachedChapters, listBookBrowserCachedChapters } from '../utils/bookChapterCache'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import BookInfoDialog from './BookInfoDialog.vue'
import RSSManager from './RSSManager.vue'
import WebDAVBrowser from './WebDAVBrowser.vue'
import ReaderBookmarkPanel from './reader/ReaderBookmarkPanel.vue'
import ReaderSearchPanel from './reader/ReaderSearchPanel.vue'
import SourceSwitchPanel from './reader/SourceSwitchPanel.vue'

const LocalStore = defineAsyncComponent(() => import('../views/LocalStore.vue'))

const router = useRouter()
const route = useRoute()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const userStore = useUserStore()

const selectedBookIds = ref([])
const batchBusy = ref(false)
const cachingBookId = ref(null)
const localCacheCounts = ref({})
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
const sourceSwitchVisible = ref(false)
const sourceSwitchBook = ref(null)
const sourceSwitchCandidates = ref([])
const sourceSwitchLoading = ref(false)
const sourceSwitchChanging = ref(null)
const sourceSwitchGroup = ref('')
const sourceSwitchQuery = ref('')
const sourceSwitchOffset = ref(0)
const sourceSwitchHasMore = ref(false)
const sourceSwitchLoadedKey = ref('')
const sourceSwitchStats = ref(null)
const contentKeyword = ref('')
const contentResults = ref([])
const contentSearching = ref(false)
const contentSearched = ref(false)
const contentLastIndex = ref(-1)
const contentHasMore = ref(false)
const contentTotal = ref(0)
const contentSearchBookKey = ref('')
const bookmarkItems = ref([])
const bookmarkLoading = ref(false)
const bookmarkEditorVisible = ref(false)
const bookmarkSaving = ref(false)
const editingBookmark = ref(null)
const bookmarkDraft = reactive({ title: '', excerpt: '', note: '' })
const backups = ref([])
const backupLoading = ref(false)
const backupListLoading = ref(false)
const restoreLoading = ref(false)
const users = ref([])
const usersLoading = ref(false)
const cleanupLoading = ref(false)
const deletingUsers = ref(false)
const creatingUser = ref(false)
const userCreateDialog = ref(false)
const selectedUserIds = ref([])
const userDraft = reactive({ username: '', password: '', role: 'user', canEditSources: true, canAccessStore: true })
const replaceRules = ref([])
const replaceRulesLoading = ref(false)
const replaceRuleImporting = ref(false)
const selectedReplaceRuleIds = ref([])
const replaceRuleFileInput = ref(null)
const replaceRuleDialog = ref(false)
const replaceRuleSaving = ref(false)
const replaceRuleTesting = ref(false)
const editingReplaceRuleId = ref(null)
const replaceRuleDraft = ref({ name: '', pattern: '', replacement: '', enabled: true })
const replaceRuleTestText = ref('广告123\n正文内容')
const replaceRuleTestResult = ref(null)
const manageKeyword = ref('')
const MINI_INTERFACE_MAX_WIDTH = 750
const windowWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)

const isMobileOverlay = computed(() => reader.pageMode === 'mobile' || windowWidth.value <= MINI_INTERFACE_MAX_WIDTH)
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
const sourceSwitchCurrentName = computed(() => {
  const sourceId = sourceSwitchBook.value?.sourceId
  if (!sourceId) return '本地书籍'
  return sourceRows.value.find(source => Number(source.id) === Number(sourceId))?.name || '当前来源'
})
const sourceSwitchGroups = computed(() => {
  const rows = sourceRows.value.length ? sourceRows.value : sourceSwitchCandidates.value
  return [...new Set(rows.map(item => item.group).filter(Boolean))].sort()
})
const bookInfoProgress = computed(() => {
  const book = overlay.bookInfoBook
  return bookProgress(book)?.percent || 0
})
const bookInfoBrowserCacheCount = computed(() => (
  overlay.bookInfoBook?.sourceId ? localCacheCount(overlay.bookInfoBook) : -1
))
const sourceStatusLabel = computed(() => overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍')
const managedBooks = computed(() => sortByShelfOrder(bookshelf.books, reader.progressByBook))
const filteredManagedBooks = computed(() => {
  const value = manageKeyword.value.trim().toLowerCase()
  if (!value) return managedBooks.value
  return managedBooks.value.filter(book => `${book.title || ''} ${book.author || ''}`.toLowerCase().includes(value))
})
const contentSearchStatus = computed(() => {
  if (!contentSearched.value) return ''
  const scanned = contentLastIndex.value >= 0 ? contentLastIndex.value + 1 : 0
  if (!contentTotal.value) return `${contentResults.value.length} 条结果`
  return `已搜索 ${Math.min(scanned, contentTotal.value)} / ${contentTotal.value} 章，${contentResults.value.length} 条结果`
})
const currentUserId = computed(() => userStore.profile?.id || null)

onMounted(() => {
  window.addEventListener('resize', updateWindowWidth, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateWindowWidth)
})

function updateWindowWidth() {
  windowWidth.value = window.innerWidth
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
    if (!visible) {
      if (!overlay.bookManageVisible) {
        manageKeyword.value = ''
        selectedBookIds.value = []
      }
      return
    }
    try {
      await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks({ all: true })])
      if (overlay.bookManageVisible) await refreshManagedBrowserCacheCounts()
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
      if (overlay.bookInfoBook?.sourceId) {
        await refreshBookInfoBrowserCacheCount(overlay.bookInfoBook)
      }
    } catch (err) {
      ElMessage.error(readError(err, '加载书籍信息失败'))
    }
  },
)

watch(
  () => overlay.searchBook?.id || overlay.searchBook?.bookUrl || '',
  (key) => {
    if (String(key || '') === contentSearchBookKey.value) return
    contentSearchBookKey.value = String(key || '')
    resetContentSearchState()
  },
)

function resetContentSearchState() {
  contentKeyword.value = ''
  contentResults.value = []
  contentSearched.value = false
  contentLastIndex.value = -1
  contentHasMore.value = false
  contentTotal.value = 0
}

watch(
  () => overlay.searchBookContentVisible,
  (visible) => {
    if (!visible) return
    const key = String(overlay.searchBook?.id || overlay.searchBook?.bookUrl || '')
    if (key && key !== contentSearchBookKey.value) {
      contentSearchBookKey.value = key
      contentKeyword.value = ''
      contentResults.value = []
      contentSearched.value = false
      contentLastIndex.value = -1
      contentHasMore.value = false
      contentTotal.value = 0
    }
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
  const progress = bookProgress(book)
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

function selectAllManagedBooks() {
  selectedBookIds.value = filteredManagedBooks.value.map(book => book.id)
}

function clearManagedSelection() {
  selectedBookIds.value = []
}

function coverInitial(book) {
  return (book?.title || '?').slice(0, 1)
}

function continueRead(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQuery(book) })
}

function goDetail(book) {
  overlay.closeBookInfo()
  overlay.bookManageVisible = false
  router.push({ name: 'book-detail', params: { id: book.id } })
}

function readerRouteQuery(book) {
  return readerRouteQueryFromBook(book, bookProgress(book))
}

function setBookGroup(book) {
  overlay.openBookGroup('set', book, {
    categoryName: categoryName(book.categoryId),
    progress: bookProgress(book)?.percent || 0,
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
      progress: bookProgress(data)?.percent || 0,
    }
    overlay.bookGroupVisible = false
    ElMessage.success('分组已设置')
  } catch (err) {
    ElMessage.error(readError(err, '设置分组失败'))
  } finally {
    settingCategorySaving.value = false
  }
}

async function refreshManagedBrowserCacheCounts() {
  const rows = managedBooks.value.filter(book => Number(book.sourceId || 0) > 0)
  try {
    localCacheCounts.value = await countBooksBrowserCachedChapters(rows)
  } catch {
    localCacheCounts.value = Object.fromEntries(rows.map(book => [book.id, 0]))
  }
}

async function refreshBookInfoBrowserCacheCount(book) {
  if (!book?.id || Number(book.sourceId || 0) <= 0) return
  try {
    const map = await listBookBrowserCachedChapters(book, book.id)
    localCacheCounts.value = {
      ...localCacheCounts.value,
      [book.id]: Object.keys(map).length,
    }
  } catch {
    localCacheCounts.value = {
      ...localCacheCounts.value,
      [book.id]: 0,
    }
  }
}

function localCacheCount(book) {
  return localCacheCounts.value[book?.id] || 0
}

function openContentSearch(book) {
  overlay.closeBookInfo()
  overlay.openSearchBookContent(book)
}

function openBookmarks(book) {
  overlay.closeBookInfo()
  overlay.openBookmark(book)
}

function openGlobalSourceSwitch(book) {
  if (!book?.id || Number(book.sourceId || 0) <= 0) return
  sourceSwitchBook.value = book
  sourceSwitchGroup.value = ''
  sourceSwitchQuery.value = ''
  sourceSwitchOffset.value = 0
  sourceSwitchHasMore.value = false
  sourceSwitchLoadedKey.value = ''
  sourceSwitchStats.value = null
  sourceSwitchCandidates.value = []
  overlay.closeBookInfo()
  sourceSwitchVisible.value = true
}

async function loadGlobalSourceCandidates({ append = false, force = false } = {}) {
  const book = sourceSwitchBook.value
  if (!book?.id || Number(book.sourceId || 0) <= 0) return
  const query = sourceSwitchQuery.value.trim()
  const key = `${book.id}:${sourceSwitchGroup.value || 'all'}:${query || 'title'}`
  if (!append && !force && sourceSwitchLoadedKey.value === key && sourceSwitchCandidates.value.length) return
  sourceSwitchLoading.value = true
  try {
    if (!sourceRows.value.length) await loadSourceRows()
    if (!append) sourceSwitchOffset.value = 0
    const { data } = await listBookSourceCandidates(book.id, {
      group: sourceSwitchGroup.value || undefined,
      q: query || undefined,
      offset: sourceSwitchOffset.value,
      limit: 10,
      paged: 1,
    })
    const rows = Array.isArray(data) ? data : (data?.list || [])
    sourceSwitchCandidates.value = append ? mergeSourceCandidates(sourceSwitchCandidates.value, rows) : rows
    sourceSwitchOffset.value = Number.isInteger(data?.nextOffset) ? data.nextOffset : sourceSwitchOffset.value + 10
    sourceSwitchHasMore.value = Boolean(data?.hasMore)
    sourceSwitchStats.value = Array.isArray(data)
      ? null
      : {
          searched: data?.searched || 0,
          matched: data?.matched || 0,
          failed: data?.failed || 0,
          empty: data?.empty || 0,
        }
    sourceSwitchLoadedKey.value = key
  } catch (err) {
    ElMessage.error(readError(err, '搜索可用来源失败'))
  } finally {
    sourceSwitchLoading.value = false
  }
}

function refreshGlobalSourceCandidates() {
  sourceSwitchLoadedKey.value = ''
  sourceSwitchHasMore.value = false
  sourceSwitchStats.value = null
  return loadGlobalSourceCandidates({ force: true })
}

function loadMoreGlobalSourceCandidates() {
  return loadGlobalSourceCandidates({ append: true })
}

function changeGlobalSourceGroup(value) {
  sourceSwitchGroup.value = value || ''
  sourceSwitchLoadedKey.value = ''
  sourceSwitchHasMore.value = false
  sourceSwitchStats.value = null
  loadGlobalSourceCandidates({ force: true })
}

function changeGlobalSourceQuery(value) {
  sourceSwitchQuery.value = value || ''
  sourceSwitchLoadedKey.value = ''
  sourceSwitchHasMore.value = false
  sourceSwitchStats.value = null
}

function mergeSourceCandidates(existing, incoming) {
  const seen = new Set(existing.map(item => `${item.sourceId}-${item.bookUrl}`))
  return existing.concat(incoming.filter(item => {
    const key = `${item.sourceId}-${item.bookUrl}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }))
}

async function changeGlobalBookSource(source) {
  const book = sourceSwitchBook.value
  if (!book?.id || source.current) return
  sourceSwitchChanging.value = source.sourceId
  try {
    const { data } = await changeBookSource(book.id, {
      sourceId: source.sourceId,
      bookUrl: source.bookUrl,
      title: source.title,
      author: source.author,
      coverUrl: source.coverUrl,
      intro: source.intro,
    })
    bookshelf.upsertBook(data)
    sourceSwitchBook.value = data
    if (overlay.bookInfoBook?.id === data.id) overlay.bookInfoBook = data
    sourceSwitchLoadedKey.value = ''
    await loadGlobalSourceCandidates({ force: true })
    ElMessage.success(`已切换到 ${source.sourceName}`)
  } catch (err) {
    ElMessage.error(readError(err, '换源失败'))
  } finally {
    sourceSwitchChanging.value = null
  }
}

function reopenSourceSwitchBookInfo() {
  if (!sourceSwitchBook.value) return
  sourceSwitchVisible.value = false
  overlay.openBookInfo(sourceSwitchBook.value, {
    categoryName: categoryName(sourceSwitchBook.value.categoryId),
    progress: bookProgress(sourceSwitchBook.value)?.percent || 0,
  })
}

async function refreshShelf() {
  loadingUpdates.value = true
  try {
    const { data } = await checkBookUpdates()
    await bookshelf.loadBooks({ force: true, all: true })
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
      await bookshelf.loadBooks({ force: true, all: true })
    }
    ElMessage.success(`目录已刷新，共 ${data?.chapterCount || updatedBook?.chapterCount || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '刷新目录失败'))
  } finally {
    refreshingBookId.value = null
  }
}

async function refreshLocalBookInfo(book) {
  if (!book?.id) return
  refreshingBookId.value = book.id
  try {
    const { data } = await refreshLocalBook(book.id)
    const updatedBook = data?.book || data
    if (updatedBook?.id) {
      bookshelf.upsertBook(updatedBook)
      overlay.bookInfoBook = updatedBook
    } else {
      await bookshelf.loadBooks({ force: true, all: true })
    }
    ElMessage.success(`本地书已刷新，共 ${data?.chapterCount || updatedBook?.chapterCount || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '刷新本地书失败'))
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
  const remoteBookIds = selectedRemoteBookIds()
  if (!remoteBookIds.length) {
    ElMessage.info('选中的本地书无需服务器缓存')
    return
  }
  batchBusy.value = true
  try {
    const data = await bookshelf.batchCacheBooks(remoteBookIds)
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
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
    const blob = await bookshelf.exportSelectedBooks(bookIds)
    downloadBlob(blob, `openreader-books-${bookIds.length}.json`)
    ElMessage.success(`已导出 ${bookIds.length} 本书`)
  } catch (err) {
    ElMessage.error(readError(err, '批量导出失败'))
  } finally {
    batchBusy.value = false
  }
}

async function cacheBook(book, command) {
  if (Number(book?.sourceId || 0) === 0) {
    ElMessage.info(command?.includes?.('Local') ? '本地书无需浏览器章节缓存' : '本地书无需服务器缓存')
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
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
    await bookshelf.loadBooks({ force: true, all: true })
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
    await bookshelf.loadBooks({ force: true, all: true })
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

async function searchAllCurrentBookContent() {
  return runCurrentBookContentSearch({ append: true, scanAll: true })
}

async function runCurrentBookContentSearch({ append = false, scanAll = false } = {}) {
  const book = overlay.searchBook
  const keyword = contentKeyword.value.trim()
  if (!book?.id || !keyword) return
  if (contentSearching.value) return
  contentSearching.value = true
  contentSearched.value = true
  try {
    let lastIndex = append ? contentLastIndex.value : -1
    let nextResults = append ? [...contentResults.value] : []
    const maxRounds = scanAll ? 80 : (append ? 1 : (Number(book.sourceId || 0) > 0 ? 4 : 1))
    let previousLastIndex = lastIndex
    for (let round = 0; round < maxRounds; round += 1) {
      const { data } = await searchBookContent(book.id, keyword, {
        paged: 1,
        lastIndex,
        scanUntilMatch: append ? 0 : 1,
        ...contentSearchPagingParams(book),
      })
      const rows = Array.isArray(data) ? data : (data?.list || [])
      nextResults = nextResults.concat(rows)
      contentResults.value = nextResults
      contentLastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : -1
      contentHasMore.value = Boolean(data?.hasMore)
      contentTotal.value = Number(data?.total || 0)
      lastIndex = contentLastIndex.value
      if (!scanAll && (rows.length || !contentHasMore.value)) break
      if (scanAll && (!contentHasMore.value || lastIndex <= previousLastIndex)) break
      previousLastIndex = lastIndex
    }
  } catch (err) {
    ElMessage.error(readError(err, '搜索正文失败'))
  } finally {
    contentSearching.value = false
  }
}

function contentSearchPagingParams(book) {
  if (Number(book?.sourceId || 0) > 0) {
    return { chapterLimit: 80, scanLimit: 240, matchLimit: 200, perChapterLimit: 20 }
  }
  return { chapterLimit: 500, scanLimit: 2000, matchLimit: 5000, perChapterLimit: 500, localFull: 1 }
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
      percent: Number.isFinite(Number(bookmark.percent)) ? Number(bookmark.percent) : undefined,
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

async function removeBookmarkItems(rows) {
  if (!Array.isArray(rows) || !rows.length) return
  try {
    await ElMessageBox.confirm(`确认要删除所选择的 ${rows.length} 条书签吗？`, '批量删除书签', { type: 'warning' })
    await Promise.all(rows.map(item => deleteBookmark(item.id)))
    const deleted = new Set(rows.map(item => item.id))
    bookmarkItems.value = bookmarkItems.value.filter(item => !deleted.has(item.id))
    ElMessage.success('书签已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除书签失败'))
  }
}

async function importBookmarkItems(rows) {
  const book = overlay.bookmarkBook
  if (!book?.id) return
  const payloads = normalizeImportedBookmarks(rows)
  if (!payloads.length) {
    ElMessage.error('书签文件没有可导入内容')
    return
  }
  try {
    await ElMessageBox.confirm(`确认要导入文件中的 ${payloads.length} 条书签到当前书籍吗？`, '导入书签', { type: 'info' })
    const created = []
    for (const payload of payloads) {
      const { data } = await createBookmark(book.id, payload)
      if (data?.id) created.push(data)
    }
    bookmarkItems.value = [...created, ...bookmarkItems.value]
    ElMessage.success(`已导入 ${created.length} 条书签`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '导入书签失败'))
  }
}

function normalizeImportedBookmarks(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map(row => {
      const chapterIndex = Math.max(0, Math.floor(Number(row.chapterIndex ?? row.durChapterIndex ?? 0)))
      return {
        chapterIndex,
        offset: Math.max(0, Math.floor(Number(row.offset ?? 0))),
        percent: clampPercent(row.percent),
        title: String(row.title || row.chapterName || row.chapterTitle || `第 ${chapterIndex + 1} 章`).trim(),
        excerpt: String(row.excerpt || row.bookText || '').trim(),
        note: String(row.note || row.content || '').trim(),
      }
    })
    .filter(row => row.title || row.excerpt || row.note)
}

function clampPercent(value) {
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : 0
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
    await bookshelf.loadBooks({ force: true, all: true })
  } catch (err) {
    ElMessage.error(readError(err, '恢复备份失败'))
  } finally {
    restoreLoading.value = false
  }
}

async function loadUsers() {
  usersLoading.value = true
  try {
    if (!userStore.profile) await userStore.loadMe()
    const { data } = await listUsers()
    users.value = data || []
    selectedUserIds.value = selectedUserIds.value.filter(id => users.value.some(user => user.id === id && isUserDeletable(user)))
  } catch (err) {
    ElMessage.error(readError(err, '加载用户失败'))
  } finally {
    usersLoading.value = false
  }
}

function isUserDeletable(user) {
  return user.role !== 'admin' && user.id !== currentUserId.value
}

function onUserSelectionChange(rows) {
  selectedUserIds.value = rows.filter(isUserDeletable).map(user => user.id)
}

function toggleUserSelection(id, checked) {
  const user = users.value.find(item => item.id === id)
  if (!user || !isUserDeletable(user)) return
  if (checked) {
    if (!selectedUserIds.value.includes(id)) selectedUserIds.value.push(id)
    return
  }
  selectedUserIds.value = selectedUserIds.value.filter(item => item !== id)
}

function openCreateUserDialog() {
  Object.assign(userDraft, {
    username: '',
    password: '',
    role: 'user',
    canEditSources: true,
    canAccessStore: true,
  })
  userCreateDialog.value = true
}

async function createManagedUser() {
  const username = userDraft.username.trim()
  if (username.length < 3 || userDraft.password.length < 6) {
    ElMessage.warning('用户名至少 3 位，密码至少 6 位')
    return
  }
  creatingUser.value = true
  try {
    await createUser({
      username,
      password: userDraft.password,
      role: userDraft.role,
      canEditSources: userDraft.canEditSources,
      canAccessStore: userDraft.canAccessStore,
    })
    ElMessage.success('新增用户成功')
    userCreateDialog.value = false
    await loadUsers()
  } catch (err) {
    ElMessage.error(readError(err, '新增用户失败'))
  } finally {
    creatingUser.value = false
  }
}

async function resetPassword(row) {
  try {
    const res = await ElMessageBox.prompt('', `重置 ${row.username} 的密码`, {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      inputType: 'password',
      inputValidator(value) {
        if (!value || value.length < 6) return '密码至少 6 位'
        return true
      },
    })
    await resetUserPassword(row.id, { password: res.value })
    ElMessage.success('重置密码成功')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重置密码失败'))
  }
}

async function deleteSelectedUsers() {
  const ids = [...selectedUserIds.value]
  if (!ids.length) {
    ElMessage.warning('请选择需要删除的用户')
    return
  }
  deletingUsers.value = true
  try {
    await ElMessageBox.confirm(`确认要删除所选择的 ${ids.length} 个用户吗？该用户空间内的书架、进度、书签和设置也会删除。`, '批量删除用户', { type: 'warning' })
    const { data } = await deleteUsers(ids)
    selectedUserIds.value = []
    ElMessage.success(`删除用户成功：${data.deleted || ids.length} 个`)
    await loadUsers()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除用户失败'))
  } finally {
    deletingUsers.value = false
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
    selectedReplaceRuleIds.value = selectedReplaceRuleIds.value.filter(id => replaceRules.value.some(rule => rule.id === id))
  } catch (err) {
    ElMessage.error(readError(err, '加载替换规则失败'))
  } finally {
    replaceRulesLoading.value = false
  }
}

function onReplaceRuleSelectionChange(rows) {
  selectedReplaceRuleIds.value = rows.map(row => row.id)
}

function toggleReplaceRuleSelection(id, checked) {
  if (checked) {
    if (!selectedReplaceRuleIds.value.includes(id)) selectedReplaceRuleIds.value.push(id)
    return
  }
  selectedReplaceRuleIds.value = selectedReplaceRuleIds.value.filter(item => item !== id)
}

function triggerReplaceRuleImport() {
  replaceRuleFileInput.value?.click()
}

async function importReplaceRuleFile(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  replaceRuleImporting.value = true
  try {
    const text = await file.text()
    const parsed = JSON.parse(text)
    const ruleList = normalizeReplaceRuleImport(parsed)
    if (!ruleList.length) {
      ElMessage.warning('替换规则文件中没有可导入的规则')
      return
    }
    await ElMessageBox.confirm(`确认要导入文件中的 ${ruleList.length} 条替换规则吗？`, '导入替换规则', { type: 'warning' })
    for (const rule of ruleList) {
      await createReplaceRule(rule)
    }
    ElMessage.success('导入替换规则成功')
    await loadReplaceRules()
    notifyReplaceRulesUpdated()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '导入替换规则失败'))
  } finally {
    replaceRuleImporting.value = false
  }
}

function normalizeReplaceRuleImport(input) {
  const rows = Array.isArray(input) ? input : Array.isArray(input?.rules) ? input.rules : []
  return rows
    .map((rule, index) => ({
      name: String(rule.name || rule.title || `导入规则 ${index + 1}`).trim(),
      pattern: String(rule.pattern || rule.regex || rule.match || '').trim(),
      replacement: String(rule.replacement ?? rule.replace ?? ''),
      enabled: rule.enabled === false || rule.isEnabled === false ? false : true,
    }))
    .filter(rule => rule.pattern)
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
    notifyReplaceRulesUpdated()
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
    notifyReplaceRulesUpdated()
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
    selectedReplaceRuleIds.value = selectedReplaceRuleIds.value.filter(id => id !== rule.id)
    ElMessage.success('替换规则已删除')
    notifyReplaceRulesUpdated()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除替换规则失败'))
  }
}

async function deleteSelectedReplaceRules() {
  const ids = [...selectedReplaceRuleIds.value]
  if (!ids.length) {
    ElMessage.warning('请选择需要删除的替换规则')
    return
  }
  try {
    await ElMessageBox.confirm(`确认要删除所选择的 ${ids.length} 条替换规则吗？`, '批量删除替换规则', { type: 'warning' })
    for (const id of ids) {
      await deleteReplaceRule(id)
    }
    replaceRules.value = replaceRules.value.filter(rule => !ids.includes(rule.id))
    selectedReplaceRuleIds.value = []
    ElMessage.success('删除替换规则成功')
    notifyReplaceRulesUpdated()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除替换规则失败'))
  }
}

function notifyReplaceRulesUpdated() {
  window.dispatchEvent(new CustomEvent('openreader:replace-rules-updated'))
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

function groupBookCount(category) {
  return managedBooks.value.filter(book => String(book.categoryId || '') === String(category.id)).length
}

async function deleteGroup(category) {
  if (groupBookCount(category) > 0) {
    ElMessage.warning('分组内还有书籍，清空后才能删除')
    return
  }
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

.group-name {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.group-name span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-name small {
  color: var(--app-text-muted);
  font-size: 12px;
}

.group-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 2px 8px;
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

  .group-create {
    grid-template-columns: 1fr;
  }

  .group-create :deep(.el-button) {
    width: 100%;
    min-height: 36px;
  }

  .group-row {
    grid-template-columns: 1fr;
    gap: 8px;
  }

  .group-row > span:first-child {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .group-actions {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    justify-content: stretch;
  }

  .group-actions :deep(.el-button) {
    min-height: 32px;
    margin-left: 0;
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
