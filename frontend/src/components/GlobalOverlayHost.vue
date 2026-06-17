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
    :show-local-refresh-action="!!overlay.bookInfoBook?.id && Number(overlay.bookInfoBook?.sourceId || 0) <= 0"
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
      <el-select
        v-if="importSupportsTocRule"
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
        v-if="importSupportsTocRule"
        v-model="importDraft.tocRule"
        type="textarea"
        :rows="2"
        placeholder="TXT目录规则（可选，留空使用默认规则，例如：^第.+章.*$）"
      />
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
        <template #default="{ row }">{{ categoryName(row.categoryId) }}</template>
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
          <el-button text class="text-button" @click="goDetail(row)">编辑</el-button>
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
            <span>{{ book.author || '未知作者' }} · {{ categoryName(book.categoryId) }}</span>
            <span>{{ Number(book.sourceId || 0) > 0 ? '远程书籍' : '本地书籍' }} · {{ progressLabel(book) }}</span>
          </button>
        </header>
        <p>共 {{ book.chapterCount || 0 }} 章<template v-if="Number(book.sourceId || 0) > 0"> · 服务器缓存 {{ serverCacheCount(book) }} 章</template> · 浏览器缓存 {{ localCacheCount(book) }} 章<template v-if="book.lastChapter"> · 最新：{{ book.lastChapter }}</template></p>
        <footer>
          <el-button size="small" text @click="goDetail(book)">编辑</el-button>
          <el-button size="small" text @click="setBookGroup(book)">分组</el-button>
          <el-button size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'cacheBookLocal')">缓存到浏览器</el-button>
          <el-button size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'deleteBookLocalCache')">清浏览器</el-button>
          <el-button v-if="Number(book.sourceId || 0) > 0" size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'cacheBook')">服务器缓存</el-button>
          <el-button v-if="Number(book.sourceId || 0) > 0" size="small" text :loading="cachingBookId === book.id" @click="cacheBook(book, 'deleteBookCache')">清服务器</el-button>
          <el-button size="small" text @click="exportBook(book, 'txt')">导出TXT</el-button>
          <el-button size="small" text @click="exportBook(book, 'epub')">导出Epub</el-button>
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
  >
    <template v-if="overlay.bookGroupMode === 'set'">
      <el-table :data="groupSetRows" row-key="id" class="group-set-table" @row-click="selectBookGroup">
        <el-table-column width="46">
          <template #default="{ row }">
            <span class="radio-cell" :class="{ active: String(settingCategoryId) === String(row.id) }" />
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
      <el-table :data="groupManageRows" row-key="id" class="group-manage-table">
        <el-table-column width="46">
          <template #default="{ row }">
            <button
              type="button"
              class="group-drag-handle"
              draggable="true"
              title="拖动排序"
              @dragstart="startGroupDrag(row)"
              @dragover.prevent
              @drop.prevent="dropGroupOn(row)"
              @dragend="finishGroupDrag"
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
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Delete, Edit, Rank, Refresh, Upload, UploadFilled } from '@element-plus/icons-vue'
import { cleanupInactiveUsers, createUser, deleteUsers, listUsers, resetUserPassword, updateUser } from '../api/admin'
import { cacheBookContent, createBookmark, deleteBookmark, listBookmarks, listChapters, listTXTTocRules, refreshLocalBook, searchBookContent, updateBook, updateBookCategory, updateBookmark } from '../api/books'
import { downloadBackup, listBackups, restoreLegadoBackup, triggerBackup } from '../api/backup'
import { createReplaceRule, deleteReplaceRule, listReplaceRules, testReplaceRule, updateReplaceRule } from '../api/replaceRules'
import { listSources } from '../api/sources'
import { uploadAsset } from '../api/uploads'
import { mergeShelfBook, useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { useUserStore } from '../stores/user'
import { bookCoverUrl, hasBookCover } from '../utils/bookCover'
import { cacheBookChaptersToBrowser, clearBookBrowserChapterCache, countBooksBrowserCachedChapters, listBookBrowserCachedChapters } from '../utils/bookChapterCache'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { localBookSearchText, normalizeLocalBookSearch } from '../utils/localBook'
import { invalidateReaderDataCache, writeReaderDataCache } from '../utils/readerDataCache'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { applyRestoreResult } from '../utils/restoreSync'
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

const selectedBookIds = ref([])
const batchBusy = ref(false)
const cachingBookId = ref(null)
const localCacheCounts = ref({})
const refreshingBookId = ref(null)
const coverUploadingBookId = ref(null)
const updatingBookId = ref(null)
const settingCategoryId = ref('')
const settingCategorySaving = ref(false)
const importingBook = ref(false)
const visibilitySavingId = ref(null)
const groupOrderDraftIds = ref([])
const draggingGroupId = ref(null)
const groupOrderSaving = ref(false)
const importDraft = reactive({ title: '', author: '', categoryId: '', file: null, tocRule: '' })
const tocRuleOptions = ref([])
const tocRulesLoading = ref(false)
const sourceRows = ref([])
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
const replaceRuleDraft = ref({ name: '', pattern: '', replacement: '', scope: '*', isRegex: false, enabled: true })
const replaceRuleTestText = ref('广告123\n正文内容')
const replaceRuleTestResult = ref(null)
const manageKeyword = ref('')
const windowWidth = ref(currentViewportWidth())
let replaceRulesRefreshTimer
let bookmarkRefreshTimer
let usersRefreshTimer
let sourceRowsRefreshTimer

const isMobileOverlay = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const importSupportsTocRule = computed(() => {
  const name = String(importDraft.file?.name || '').toLowerCase()
  return /\.(txt|text|md)$/.test(name)
})
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
  return bookProgress(book)?.percent || 0
})
const bookInfoBrowserCacheCount = computed(() => (
  overlay.bookInfoBook?.id ? localCacheCount(overlay.bookInfoBook) : -1
))
const sourceStatusLabel = computed(() => overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍')
const groupSetRows = computed(() => [
  { id: '', name: '未分组', description: '从当前分组移出' },
  ...bookshelf.categories.map(category => ({
    ...category,
    id: String(category.id),
    description: `${groupBookCount(category)} 本`,
  })),
])
const groupManageRows = computed(() => {
  const categoryById = new Map(bookshelf.categories.map(category => [String(category.id), category]))
  const rows = []
  for (const id of groupOrderDraftIds.value) {
    const category = categoryById.get(String(id))
    if (category) rows.push(category)
  }
  for (const category of bookshelf.categories) {
    if (!groupOrderDraftIds.value.includes(String(category.id))) rows.push(category)
  }
  return rows
})
const isGroupOrderDirty = computed(() => (
  groupManageRows.value.map(category => String(category.id)).join(',') !== bookshelf.categories.map(category => String(category.id)).join(',')
))
const managedBooks = computed(() => sortByShelfOrder(bookshelf.books, reader.progressByBook))
const filteredManagedBooks = computed(() => {
  const value = normalizeLocalBookSearch(manageKeyword.value)
  if (!value) return managedBooks.value
  return managedBooks.value.filter(book => manageBookSearchText(book).includes(value))
})

function manageBookSearchText(book) {
  return localBookSearchText(book, [
    progressLabel(book),
    categoryName(book.categoryId),
  ])
}
const contentSearchStatus = computed(() => {
  if (!contentSearched.value) return ''
  const scanned = contentLastIndex.value >= 0 ? contentLastIndex.value + 1 : 0
  if (!contentTotal.value) return `${contentResults.value.length} 条结果`
  return `已搜索 ${Math.min(scanned, contentTotal.value)} / ${contentTotal.value} 章，${contentResults.value.length} 条结果`
})
const currentUserId = computed(() => userStore.profile?.id || null)

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
  clearBookmarkRefreshTimer()
  clearUsersRefreshTimer()
  clearSourceRowsRefreshTimer()
})

function updateWindowWidth() {
  windowWidth.value = currentViewportWidth()
}

async function loadImportCategories() {
  try {
    await warmOverlayCategories()
  } catch (err) {
    ElMessage.error(readError(err, '加载分组失败'))
  }
}

async function loadTXTTocRuleOptions() {
  if (tocRuleOptions.value.length || tocRulesLoading.value) return
  tocRulesLoading.value = true
  try {
    const { data } = await listTXTTocRules()
    tocRuleOptions.value = Array.isArray(data) ? data.filter(rule => rule?.enable !== false && rule?.rule) : []
  } catch (err) {
    ElMessage.error(readError(err, '加载目录规则失败'))
  } finally {
    tocRulesLoading.value = false
  }
}

function pickImportFile(data) {
  importDraft.file = data.raw || null
  if (!importSupportsTocRule.value) importDraft.tocRule = ''
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
      tocRule: importSupportsTocRule.value ? importDraft.tocRule : '',
    })
    ElMessage.success(`已导入《${book.title}》，共 ${book.chapterCount || 0} 章`)
    Object.assign(importDraft, { title: '', author: '', categoryId: '', file: null, tocRule: '' })
    overlay.importBookVisible = false
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importingBook.value = false
  }
}

watch(
  importSupportsTocRule,
  (supported) => {
    if (supported) loadTXTTocRuleOptions()
    else importDraft.tocRule = ''
  },
)

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
    if (overlay.bookGroupVisible && overlay.bookGroupMode === 'set') {
      settingCategoryId.value = overlay.bookInfoBook?.categoryId ? String(overlay.bookInfoBook.categoryId) : ''
    } else if (overlay.bookGroupVisible) {
      resetGroupOrderDraft()
    }
  },
)

watch(
  () => overlay.bookInfoVisible,
  async (visible) => {
    if (!visible) return
    await warmOverlayCategories().catch((err) => {
      ElMessage.warning(readError(err, '分组加载失败，书籍信息仍可查看'))
    })
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

function goDetail(book) {
  overlay.closeBookInfo()
  overlay.bookManageVisible = false
  router.push({ name: 'book-detail', params: { id: book.id } })
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
  const rows = managedBooks.value.filter(book => book?.id)
  try {
    localCacheCounts.value = await countBooksBrowserCachedChapters(rows)
  } catch {
    localCacheCounts.value = Object.fromEntries(rows.map(book => [book.id, 0]))
  }
}

async function refreshBookInfoBrowserCacheCount(book) {
  if (!book?.id) return
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

async function invalidateBookReaderCaches(book, options = {}) {
  if (!book?.id) return
  await invalidateReaderDataCache(book.id, { book: true, chapters: true })
  if (options.clearBrowser) {
    await clearBookBrowserChapterCache(book, book.id).catch(() => 0)
    localCacheCounts.value = {
      ...localCacheCounts.value,
      [book.id]: 0,
    }
  }
}

async function refreshBookChaptersCache(book) {
  if (!book?.id) return null
  try {
    const { data } = await listChapters(book.id)
    const chapters = Array.isArray(data) ? data : []
    await writeReaderDataCache(book.id, { bookData: book, chaptersData: chapters })
    return chapters
  } catch {
    await writeReaderDataCache(book.id, { bookData: book })
    return null
  }
}

function mergedShelfBook(book) {
  if (!book?.id) return book
  const current = bookshelf.books.find(item => Number(item.id) === Number(book.id)) ||
    (Number(overlay.bookInfoBook?.id) === Number(book.id) ? overlay.bookInfoBook : null)
  return mergeShelfBook(current, book)
}

function applyUpdatedBookToOverlay(book, chapters = null) {
  if (!book?.id) return book
  const nextBook = mergedShelfBook(book)
  bookshelf.upsertBook(nextBook)
  if (Number(overlay.bookInfoBook?.id) === Number(nextBook.id)) overlay.bookInfoBook = nextBook
  window.dispatchEvent(new CustomEvent('openreader:reader-book-data-updated', {
    detail: { bookId: nextBook.id, book: nextBook, chapters },
  }))
  return nextBook
}

function localCacheCount(book) {
  return localCacheCounts.value[book?.id] || 0
}

function serverCacheCount(book) {
  return Number(book?.cachedChapterCount || 0)
}

function updateServerCacheCount(book, count) {
  if (!book?.id) return
  const nextCount = Math.max(0, Number(count || 0))
  const nextBook = { ...book, cachedChapterCount: nextCount }
  bookshelf.upsertBook(nextBook)
  if (Number(overlay.bookInfoBook?.id) === Number(book.id)) {
    overlay.bookInfoBook = { ...overlay.bookInfoBook, cachedChapterCount: nextCount }
  }
}

async function refreshLocalBookInfo(book) {
  if (!book?.id) return
  refreshingBookId.value = book.id
  try {
    const { data } = await refreshLocalBook(book.id)
    await invalidateBookReaderCaches(book, { clearBrowser: true })
    const updatedBook = data?.book || data
    if (updatedBook?.id) {
      const mergedBook = mergedShelfBook(updatedBook)
      const chapters = await refreshBookChaptersCache(mergedBook)
      applyUpdatedBookToOverlay(mergedBook, chapters)
      await refreshBookInfoBrowserCacheCount(mergedBook)
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

async function uploadBookInfoCover(file) {
  const book = overlay.bookInfoBook
  if (!book?.id || !file) return
  coverUploadingBookId.value = book.id
  try {
    const { data: uploadResult } = await uploadAsset({ file, type: 'cover' })
    const { data: updatedBook } = await updateBook(book.id, {
      title: book.title,
      author: book.author || '',
      customCoverUrl: uploadResult.url,
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

function handleBookmarksUpdated(event) {
  if (!overlay.bookmarkVisible || !overlay.bookmarkBook?.id) return
  const bookIds = event?.detail?.bookIds || []
  if (bookIds.length && !bookIds.some(id => String(id) === String(overlay.bookmarkBook.id))) return
  scheduleBookmarkRefresh()
}

function scheduleBookmarkRefresh() {
  clearBookmarkRefreshTimer()
  bookmarkRefreshTimer = window.setTimeout(async () => {
    bookmarkRefreshTimer = undefined
    await loadBookmarkItems()
  }, 250)
}

function clearBookmarkRefreshTimer() {
  if (!bookmarkRefreshTimer) return
  window.clearTimeout(bookmarkRefreshTimer)
  bookmarkRefreshTimer = undefined
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
    ElMessage.success(`备份已保存到 WebDAV：${data.name || data.path || 'backup.zip'}`)
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
    await applyRestoreResult(result)
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

function handleUsersUpdated() {
  if (!overlay.userManageVisible) return
  scheduleUsersRefresh()
}

function scheduleUsersRefresh() {
  clearUsersRefreshTimer()
  usersRefreshTimer = window.setTimeout(async () => {
    usersRefreshTimer = undefined
    await loadUsers()
  }, 250)
}

function clearUsersRefreshTimer() {
  if (!usersRefreshTimer) return
  window.clearTimeout(usersRefreshTimer)
  usersRefreshTimer = undefined
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
    replaceRules.value = Array.isArray(data) ? data.map(normalizeReplaceRule) : []
    selectedReplaceRuleIds.value = selectedReplaceRuleIds.value.filter(id => replaceRules.value.some(rule => rule.id === id))
  } catch (err) {
    ElMessage.error(readError(err, '加载替换规则失败'))
  } finally {
    replaceRulesLoading.value = false
  }
}

function handleReplaceRulesUpdated(event) {
  if (event?.detail?.local || !overlay.replaceRulesVisible) return
  scheduleReplaceRulesRefresh()
}

function scheduleReplaceRulesRefresh() {
  clearReplaceRulesRefreshTimer()
  replaceRulesRefreshTimer = window.setTimeout(async () => {
    replaceRulesRefreshTimer = undefined
    await loadReplaceRules()
  }, 250)
}

function clearReplaceRulesRefreshTimer() {
  if (!replaceRulesRefreshTimer) return
  window.clearTimeout(replaceRulesRefreshTimer)
  replaceRulesRefreshTimer = undefined
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
      scope: String(rule.scope || '*').trim() || '*',
      isRegex: rule.isRegex === true,
      enabled: rule.enabled === false || rule.isEnabled === false ? false : true,
    }))
    .filter(rule => rule.pattern)
}

function normalizeReplaceRule(rule = {}) {
  rule = rule || {}
  return {
    ...rule,
    scope: String(rule.scope || '*').trim() || '*',
    isRegex: rule.isRegex == null ? true : rule.isRegex === true,
    enabled: rule.enabled === false || rule.isEnabled === false ? false : true,
  }
}

function openReplaceRuleEditor(rule = null) {
  if (!rule) {
    editingReplaceRuleId.value = null
    replaceRuleDraft.value = { name: '', pattern: '', replacement: '', scope: '*', isRegex: false, enabled: true }
    replaceRuleTestResult.value = null
    replaceRuleDialog.value = true
    return
  }
  const normalized = normalizeReplaceRule(rule)
  editingReplaceRuleId.value = normalized.id || null
  replaceRuleDraft.value = {
    name: normalized.name || '',
    pattern: normalized.pattern || '',
    replacement: normalized.replacement || '',
    scope: normalized.scope || '*',
    isRegex: normalized.isRegex,
    enabled: normalized.enabled,
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
    const payload = normalizeReplaceRule({
      ...replaceRuleDraft.value,
      pattern: replaceRuleDraft.value.pattern.trim(),
      scope: replaceRuleDraft.value.scope,
    })
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

async function toggleReplaceRule(rule, enabled) {
  const normalized = normalizeReplaceRule({ ...rule, enabled })
  try {
    await updateReplaceRule(normalized.id, {
      name: normalized.name,
      pattern: normalized.pattern,
      replacement: normalized.replacement,
      scope: normalized.scope,
      isRegex: normalized.isRegex,
      enabled: normalized.enabled,
    })
    rule.enabled = normalized.enabled
    rule.isEnabled = normalized.enabled
    ElMessage.success(normalized.enabled ? '规则已启用' : '规则已停用')
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
      isRegex: replaceRuleDraft.value.isRegex,
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
  window.dispatchEvent(new CustomEvent('openreader:replace-rules-updated', { detail: { local: true } }))
}

async function createCategory() {
  try {
    const { value } = await ElMessageBox.prompt('输入分组名称', '添加分组', {
      inputValidator: value => !!value?.trim() || '分组名称不能为空',
    })
    const name = value.trim()
    if (!name) return
    await bookshelf.addCategory({ name })
    resetGroupOrderDraft()
    ElMessage.success('分组已创建')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
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
    resetGroupOrderDraft()
    ElMessage.success('分组已重命名')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名失败'))
  }
}

function groupBookCount(category) {
  return managedBooks.value.filter(book => String(book.categoryId || '') === String(category.id)).length
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

async function toggleGroupVisibility(category, show) {
  visibilitySavingId.value = category.id
  try {
    await bookshelf.setCategoryVisible(category.id, show)
    ElMessage.success(show ? '分组已显示' : '分组已隐藏')
  } catch (err) {
    await bookshelf.loadCategories({ force: true }).catch(() => {})
    ElMessage.error(readError(err, '修改分组显示状态失败'))
  } finally {
    visibilitySavingId.value = null
  }
}

async function deleteGroup(category) {
  if (groupBookCount(category) > 0) {
    ElMessage.warning('分组内还有书籍，清空后才能删除')
    return
  }
  try {
    await ElMessageBox.confirm(`确定删除分组“${category.name}”吗？`, '删除分组', { type: 'warning' })
    await bookshelf.removeCategory(category.id)
    resetGroupOrderDraft()
    ElMessage.success('分组已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除分组失败'))
  }
}

function resetGroupOrderDraft() {
  groupOrderDraftIds.value = bookshelf.categories.map(category => String(category.id))
  draggingGroupId.value = null
}

function startGroupDrag(category) {
  draggingGroupId.value = String(category.id)
}

function finishGroupDrag() {
  draggingGroupId.value = null
}

function dropGroupOn(targetCategory) {
  const sourceId = draggingGroupId.value
  const targetId = String(targetCategory.id)
  if (!sourceId || sourceId === targetId) return
  const ids = groupManageRows.value.map(category => String(category.id))
  const sourceIndex = ids.indexOf(sourceId)
  const targetIndex = ids.indexOf(targetId)
  if (sourceIndex < 0 || targetIndex < 0) return
  const [moved] = ids.splice(sourceIndex, 1)
  ids.splice(targetIndex, 0, moved)
  groupOrderDraftIds.value = ids
}

async function saveGroupOrderDraft() {
  if (!isGroupOrderDirty.value) return
  const orderedIds = groupManageRows.value.map(item => item.id)
  groupOrderSaving.value = true
  try {
    await bookshelf.reorderCategoryIds(orderedIds)
    resetGroupOrderDraft()
    ElMessage.success('分组排序已更新')
  } catch (err) {
    ElMessage.error(readError(err, '分组排序失败'))
  } finally {
    groupOrderSaving.value = false
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
