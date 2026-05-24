<template>
  <section class="app-page settings-page">
    <header class="settings-head">
      <div>
        <p class="eyebrow">Settings</p>
        <h1 class="app-page-title">设置</h1>
        <p class="app-page-subtitle">账户、同步、备份、WebDAV 和管理员能力集中在这里。</p>
      </div>
      <el-button :icon="Refresh" :loading="checking" @click="checkHealth">检查服务</el-button>
    </header>

    <el-tabs v-model="activeTab" class="settings-tabs">
      <el-tab-pane label="账户" name="account">
        <section class="settings-grid">
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><User /></el-icon>
              <h2>账户</h2>
            </div>
            <dl class="info-list">
              <div><dt>用户名</dt><dd>{{ userStore.profile?.username || '-' }}</dd></div>
              <div><dt>角色</dt><dd>{{ userStore.profile?.role || '-' }}</dd></div>
              <div><dt>书籍限制</dt><dd>{{ limitText(userStore.profile?.bookLimit) }}</dd></div>
              <div><dt>书源限制</dt><dd>{{ limitText(userStore.profile?.sourceLimit) }}</dd></div>
            </dl>
            <el-button type="primary" plain :icon="SwitchButton" @click="logout">退出登录</el-button>
          </article>

          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Connection /></el-icon>
              <h2>同步</h2>
            </div>
            <p class="panel-text">阅读进度和书架变更通过 WebSocket 推送。当前连接状态：</p>
            <el-tag :type="syncConnected ? 'success' : 'info'" effect="plain">
              {{ syncConnected ? '同步在线' : '等待连接' }}
            </el-tag>
            <dl v-if="healthInfo" class="info-list service-info">
              <div><dt>构建时间</dt><dd>{{ healthInfo.buildDate || '-' }}</dd></div>
              <div><dt>提交版本</dt><dd>{{ shortCommit(healthInfo.commit) }}</dd></div>
            </dl>
          </article>
        </section>
      </el-tab-pane>

      <el-tab-pane label="备份" name="backup">
        <section class="app-panel settings-card">
          <div class="card-head">
            <el-icon><Files /></el-icon>
            <h2>备份恢复</h2>
          </div>
          <div class="panel-actions">
            <el-button type="primary" :icon="Upload" :loading="backupLoading" @click="runBackup">保存备份</el-button>
            <el-upload :show-file-list="false" :auto-upload="false" accept=".zip" @change="restoreBackup">
              <el-button :icon="RefreshLeft" :loading="restoreLoading">恢复 Legado 备份</el-button>
            </el-upload>
            <el-button :icon="Refresh" :loading="backupListLoading" @click="loadBackups">刷新列表</el-button>
          </div>

          <el-table :data="backups" stripe class="backup-table desktop-backup-table">
            <el-table-column prop="name" label="文件名" min-width="220" show-overflow-tooltip />
            <el-table-column label="大小" width="110">
              <template #default="{ row }">{{ formatSize(row.size) }}</template>
            </el-table-column>
            <el-table-column label="时间" width="190">
              <template #default="{ row }">{{ formatDate(row.time) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button text type="primary" @click="download(row)">下载</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="backups.length" class="mobile-backup-list">
            <article v-for="row in backups" :key="row.name" class="mobile-backup-card app-panel">
              <div>
                <strong>{{ row.name }}</strong>
                <span>{{ formatDate(row.time) }} · {{ formatSize(row.size) }}</span>
              </div>
              <el-button size="small" text type="primary" @click="download(row)">下载</el-button>
            </article>
          </div>
          <el-empty v-if="!backups.length && !backupListLoading" description="暂无备份文件" />
        </section>
      </el-tab-pane>

      <el-tab-pane label="缓存" name="cache">
        <section class="settings-grid">
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Files /></el-icon>
              <h2>远程章节缓存</h2>
            </div>
            <p class="panel-text">这里只统计远程书源章节缓存。本地书导入后正文存放在书库目录，不计入这里。</p>
            <dl class="info-list">
              <div><dt>缓存目录</dt><dd>{{ cacheStats.path || '-' }}</dd></div>
              <div><dt>缓存文件</dt><dd>{{ cacheStats.files || 0 }}</dd></div>
              <div><dt>缓存大小</dt><dd>{{ formatSize(cacheStats.size || 0) }}</dd></div>
              <div><dt>章节状态</dt><dd>{{ cacheStats.cachedChapters || 0 }} 章已缓存</dd></div>
            </dl>
            <div class="panel-actions">
              <el-button :icon="Refresh" :loading="cacheLoading" @click="loadCacheStats">刷新</el-button>
              <el-button type="danger" plain :icon="Delete" :loading="cacheClearing" @click="clearSystemCache">清理缓存</el-button>
            </div>
          </article>
        </section>
      </el-tab-pane>

      <el-tab-pane label="WebDAV" name="webdav">
        <section class="app-panel settings-card">
          <div class="card-head">
            <el-icon><Link /></el-icon>
            <h2>WebDAV</h2>
          </div>
          <dl class="info-list">
            <div><dt>服务地址</dt><dd><code>/webdav/</code></dd></div>
            <div><dt>当前目录</dt><dd>{{ webdavPath || '/' }}</dd></div>
          </dl>
          <div class="panel-actions">
            <el-button :icon="Refresh" :loading="webdavLoading" @click="loadWebDAV">刷新</el-button>
            <el-button :icon="FolderOpened" @click="createWebDAVFolder">新建目录</el-button>
            <el-upload :show-file-list="false" :auto-upload="false" @change="uploadWebDAVFile">
              <el-button :icon="Upload" :loading="webdavUploading">上传</el-button>
            </el-upload>
            <el-button type="danger" plain :disabled="!webdavSelection.length" @click="deleteSelectedWebDAVItems">
              批量删除 ({{ webdavSelection.length }})
            </el-button>
            <el-button type="primary" :disabled="!webdavImportSelection.length" :loading="webdavImporting" @click="importSelectedWebDAVBooks">
              批量加入书架 ({{ webdavImportSelection.length }})
            </el-button>
          </div>
          <el-breadcrumb separator="/" class="webdav-breadcrumb">
            <el-breadcrumb-item>
              <button type="button" @click="goWebDAVPath('')">webdav</button>
            </el-breadcrumb-item>
            <el-breadcrumb-item v-for="crumb in webdavBreadcrumbs" :key="crumb.path">
              <button type="button" @click="goWebDAVPath(crumb.path)">{{ crumb.name }}</button>
            </el-breadcrumb-item>
          </el-breadcrumb>
          <el-table :data="webdavItems" stripe v-loading="webdavLoading" class="webdav-table desktop-webdav-table" @selection-change="webdavSelection = $event">
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
            <el-table-column label="操作" width="260" fixed="right">
              <template #default="{ row }">
                <el-button v-if="!row.isDir && isBackupFile(row)" text type="primary" :loading="webdavRestoring === row.name" @click="restoreWebDAVBackupFile(row)">恢复</el-button>
                <el-button v-if="!row.isDir" text type="primary" @click="downloadWebDAVFile(row)">下载</el-button>
                <el-button v-if="row.importable" text type="primary" :loading="webdavImporting" @click="importWebDAVBook(row)">加入书架</el-button>
                <el-button text @click="renameWebDAVItem(row)">重命名</el-button>
                <el-button text type="danger" @click="deleteWebDAVItem(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="webdavItems.length" class="mobile-file-select-actions app-panel">
            <span>已选 {{ webdavSelection.length }} 个</span>
            <div>
              <el-button size="small" text @click="selectShownWebDAVFiles">全选当前</el-button>
              <el-button size="small" text @click="webdavSelection = []">清空</el-button>
            </div>
          </div>
          <div v-if="webdavItems.length" v-loading="webdavLoading" class="mobile-file-list">
            <article v-for="row in webdavItems" :key="row.name" class="mobile-file-card app-panel">
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
              <div class="mobile-file-meta">
                <el-tag size="small" effect="plain">{{ row.isDir ? '目录' : '文件' }}</el-tag>
                <el-tag v-if="row.importable" size="small" type="success" effect="plain">可加入书架</el-tag>
                <el-tag v-if="!row.isDir && isBackupFile(row)" size="small" type="warning" effect="plain">备份</el-tag>
              </div>
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
      </el-tab-pane>

      <el-tab-pane label="阅读" name="reader">
        <section class="settings-grid">
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><View /></el-icon>
              <h2>阅读默认值</h2>
            </div>
            <div class="reader-setting-list">
              <label>
                <span>阅读模式</span>
                <el-radio-group v-model="readerModeModel" size="small" @change="readerStore.setMode($event)">
                  <el-radio-button value="scroll">滚动</el-radio-button>
                  <el-radio-button value="flip">左右翻页</el-radio-button>
                  <el-radio-button value="page">上下分页</el-radio-button>
                </el-radio-group>
              </label>
              <label>
                <span>字体</span>
                <el-select v-model="readerFontFamilyModel" size="small" @change="readerStore.setFontFamily($event)">
                  <el-option v-for="font in fontOptions" :key="font.value" :label="font.label" :value="font.value" />
                </el-select>
              </label>
              <label>
                <span>亮度 {{ readerStore.brightness }}%</span>
                <el-slider v-model="readerBrightnessModel" :min="50" :max="150" @input="readerStore.setBrightness($event)" @change="readerStore.setBrightness($event)" />
              </label>
              <label>
                <span>自动阅读速度 {{ readerStore.autoReadSpeed }}px</span>
                <el-slider v-model="readerAutoReadSpeedModel" :min="2" :max="40" :step="1" @input="readerStore.setAutoReadSpeed($event)" @change="readerStore.setAutoReadSpeed($event)" />
              </label>
              <label>
                <span>字号 {{ readerStore.fontSize }}px</span>
                <el-slider v-model="readerFontSizeModel" :min="8" :max="36" @input="readerStore.setFontSize($event)" @change="readerStore.setFontSize($event)" />
              </label>
              <label>
                <span>字重 {{ readerStore.fontWeight }}</span>
                <el-slider v-model="readerFontWeightModel" :min="300" :max="900" :step="100" @input="readerStore.setFontWeight($event)" @change="readerStore.setFontWeight($event)" />
              </label>
              <label>
                <span>行高 {{ readerStore.lineHeight }}</span>
                <el-slider v-model="readerLineHeightModel" :min="1" :max="5" :step="0.2" @input="readerStore.setLineHeight($event)" @change="readerStore.setLineHeight($event)" />
              </label>
              <label>
                <span>段落间距 {{ readerStore.paragraphSpace }}em</span>
                <el-slider v-model="readerParagraphSpaceModel" :min="0" :max="3" :step="0.1" @input="readerStore.setParagraphSpace($event)" @change="readerStore.setParagraphSpace($event)" />
              </label>
              <label>
                <span>阅读宽度 {{ readerStore.columnWidth }}px</span>
                <el-slider v-model="readerColumnWidthModel" :min="560" :max="1080" :step="20" @input="readerStore.setColumnWidth($event)" @change="readerStore.setColumnWidth($event)" />
              </label>
              <label>
                <span>朗读语速 {{ readerStore.ttsRate }}</span>
                <el-slider v-model="readerTTSRateModel" :min="0.5" :max="3" :step="0.1" @input="readerStore.setTTSRate($event)" @change="readerStore.setTTSRate($event)" />
              </label>
              <label>
                <span>朗读音调 {{ readerStore.ttsPitch }}</span>
                <el-slider v-model="readerTTSPitchModel" :min="0.5" :max="2" :step="0.1" @input="readerStore.setTTSPitch($event)" @change="readerStore.setTTSPitch($event)" />
              </label>
            </div>
          </article>
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Moon /></el-icon>
              <h2>主题</h2>
            </div>
            <div class="theme-list">
              <button
                v-for="(theme, key) in themePresets"
                :key="key"
                type="button"
                class="theme-choice"
                :class="{ active: readerStore.theme === key }"
                @click="readerStore.setTheme(key)"
              >
                <span class="theme-swatch" :style="{ background: theme.bg }" />
                <span>{{ theme.label }}</span>
              </button>
              <button type="button" class="theme-choice" :class="{ active: readerStore.theme === 'custom' }" @click="readerStore.setTheme('custom')">
                <span class="theme-swatch custom-swatch" :style="{ background: readerStore.customBgColor || '#f4e9bd' }" />
                <span>自定义</span>
              </button>
            </div>
            <div v-if="readerStore.theme === 'custom'" class="custom-theme-row">
              <span>背景色</span>
              <el-color-picker v-model="readerStore.customBgColor" @change="readerStore.setCustomBgColor($event)" />
              <el-upload accept="image/*" :show-file-list="false" :auto-upload="false" @change="pickReaderBgImage">
                <el-button size="small" :icon="Upload" :loading="readerBgUploading">背景图</el-button>
              </el-upload>
              <el-button v-if="readerStore.customBgImage" size="small" text type="danger" @click="readerStore.setCustomBgImage('')">清除背景图</el-button>
            </div>
          </article>
        </section>
      </el-tab-pane>

      <el-tab-pane label="替换规则" name="replace">
        <section class="app-panel settings-card">
          <div class="card-head">
            <el-icon><Edit /></el-icon>
            <h2>全局替换规则</h2>
          </div>
          <div class="panel-actions">
            <el-button type="primary" :icon="Edit" @click="openReplaceRuleEditor()">新增规则</el-button>
            <el-button :icon="Refresh" :loading="replaceRulesLoading" @click="loadReplaceRules">刷新</el-button>
          </div>
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
            <article v-for="rule in replaceRules" :key="rule.id" class="mobile-rule-card app-panel">
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
      </el-tab-pane>

      <el-tab-pane label="RSS" name="rss">
        <section class="settings-grid rss-grid">
          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Connection /></el-icon>
              <h2>RSS 源</h2>
            </div>
            <div class="panel-actions">
              <el-button type="primary" :icon="Edit" @click="openRSSEditor()">新增 RSS</el-button>
              <el-button :icon="Refresh" :loading="rssSourcesLoading" @click="loadRSSSources">刷新</el-button>
            </div>
            <div class="rss-source-list">
              <div v-for="source in rssSources" :key="source.id" class="rss-source-row" :class="{ active: selectedRSSSourceId === source.id }">
                <button type="button" @click="selectRSSSource(source.id)">
                  <strong>{{ source.title }}</strong>
                  <small>{{ source.url }}</small>
                </button>
                <span>
                  <el-button text size="small" :loading="rssRefreshing === source.id" @click="refreshRSS(source)">刷新</el-button>
                  <el-button text size="small" @click="openRSSEditor(source)">编辑</el-button>
                  <el-button text size="small" type="danger" @click="removeRSSSource(source)">删除</el-button>
                </span>
              </div>
            </div>
            <el-empty v-if="!rssSourcesLoading && !rssSources.length" description="暂无 RSS 源" />
          </article>

          <article class="app-panel settings-card">
            <div class="card-head">
              <el-icon><Document /></el-icon>
              <h2>文章</h2>
            </div>
            <div class="panel-actions">
              <el-button :icon="Refresh" :loading="rssArticlesLoading" @click="loadRSSArticles">刷新文章</el-button>
              <el-radio-group v-model="rssArticleFilter" size="small" @change="loadRSSArticles">
                <el-radio-button value="all">全部</el-radio-button>
                <el-radio-button value="unread">未读</el-radio-button>
                <el-radio-button value="favorite">收藏</el-radio-button>
              </el-radio-group>
            </div>
            <div class="rss-article-list">
              <article v-for="article in rssArticles" :key="article.id" class="rss-article" :class="{ read: article.isRead }">
                <div class="rss-article-line">
                  <button type="button" class="rss-article-title" @click="openRSSArticle(article)">{{ article.title }}</button>
                  <el-button
                    text
                    size="small"
                    :type="article.favorite ? 'warning' : 'info'"
                    @click="toggleRSSFavorite(article)"
                  >
                    {{ article.favorite ? '已收藏' : '收藏' }}
                  </el-button>
                </div>
                <small>{{ formatDate(article.publishedAt || article.updatedAt) }} · {{ article.author || '未知作者' }}</small>
                <p>{{ article.summary || article.content || '无摘要' }}</p>
              </article>
            </div>
            <el-empty v-if="!rssArticlesLoading && !rssArticles.length" description="暂无 RSS 文章" />
          </article>
        </section>
      </el-tab-pane>

      <el-tab-pane label="用户管理" name="admin">
        <section class="app-panel settings-card">
          <div class="card-head">
            <el-icon><UserFilled /></el-icon>
            <h2>用户空间</h2>
          </div>
          <div class="panel-actions">
            <el-button :icon="Refresh" :loading="usersLoading" @click="loadUsers">加载用户</el-button>
            <el-button :icon="Delete" :loading="cleanupLoading" @click="cleanupInactive">清理不活跃用户</el-button>
          </div>
          <el-table :data="users" stripe class="desktop-user-table">
            <el-table-column prop="username" label="用户名" min-width="140" />
            <el-table-column prop="role" label="角色" width="90" />
            <el-table-column prop="bookCount" label="书籍" width="80" />
            <el-table-column prop="sourceCount" label="书源" width="80" />
            <el-table-column label="权限" min-width="260">
              <template #default="{ row }">
                <div class="permission-row">
                  <el-switch v-model="row.canEditSources" size="small" active-text="书源" @change="updateUserPermission(row)" />
                  <el-switch v-model="row.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(row)" />
                </div>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="users.length" v-loading="usersLoading" class="mobile-user-list">
            <article v-for="user in users" :key="user.id" class="mobile-user-card app-panel">
              <header>
                <div>
                  <strong>{{ user.username }}</strong>
                  <span>{{ user.role }} · 书籍 {{ user.bookCount || 0 }} · 书源 {{ user.sourceCount || 0 }}</span>
                </div>
              </header>
              <div class="mobile-permission-row">
                <el-switch v-model="user.canEditSources" size="small" active-text="书源" @change="updateUserPermission(user)" />
                <el-switch v-model="user.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(user)" />
              </div>
            </article>
          </div>
          <el-alert type="warning" :closable="false" show-icon title="只有管理员账号能访问用户管理接口；普通账号加载失败是预期行为。" />
        </section>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="replaceRuleDialog" :title="editingReplaceRuleId ? '编辑替换规则' : '新增替换规则'" width="520px" :fullscreen="isMobileDialog">
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

    <el-dialog v-model="rssDialog" :title="editingRSSSourceId ? '编辑 RSS 源' : '新增 RSS 源'" width="520px" :fullscreen="isMobileDialog">
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

    <el-dialog v-model="rssArticleDialog" title="RSS 文章" width="720px" class="rss-reader-dialog" :fullscreen="isMobileDialog">
      <article v-if="selectedRSSArticle" class="rss-reader">
        <h2>{{ selectedRSSArticle.title }}</h2>
        <small>{{ formatDate(selectedRSSArticle.publishedAt || selectedRSSArticle.updatedAt) }} · {{ selectedRSSArticle.author || '未知作者' }}</small>
        <p>{{ rssArticleBody(selectedRSSArticle) }}</p>
      </article>
      <template #footer>
        <el-button @click="rssArticleDialog = false">关闭</el-button>
        <el-button v-if="selectedRSSArticle?.link" type="primary" @click="openExternal(selectedRSSArticle.link)">打开原文</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="webdavImportResultDialog" title="WebDAV 导入结果" width="560px" :fullscreen="isMobileDialog">
      <div class="result-list">
        <div v-for="(item, index) in webdavImportResults" :key="index" class="result-row">
          <el-tag :type="item.book ? 'success' : 'danger'" effect="plain">{{ item.book ? '成功' : '失败' }}</el-tag>
          <span>{{ item.book?.title || item.path }}</span>
          <small>{{ item.error || `${item.book?.chapterCount || 0} 章` }}</small>
        </div>
      </div>
    </el-dialog>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Connection,
  Delete,
  Document,
  Edit,
  Files,
  FolderOpened,
  Link,
  Refresh,
  RefreshLeft,
  SwitchButton,
  Upload,
  User,
  UserFilled,
  View,
  Moon,
} from '@element-plus/icons-vue'
import api from '../api/client'
import { cleanupInactiveUsers, listUsers, updateUser } from '../api/admin'
import { downloadBackup, listBackups, restoreLegadoBackup, restoreWebDAVBackup, triggerBackup } from '../api/backup'
import { clearCache, getCacheStats } from '../api/cache'
import { createReplaceRule, deleteReplaceRule, listReplaceRules, testReplaceRule, updateReplaceRule } from '../api/replaceRules'
import { createRSSSource, deleteRSSSource, listRSSArticles, listRSSSources, refreshRSSSource, updateRSSArticle, updateRSSSource } from '../api/rss'
import { uploadAsset } from '../api/uploads'
import { createWebDAVDirectory, deleteWebDAV, downloadWebDAV, importFromWebDAV, listWebDAV, renameWebDAV, uploadWebDAV } from '../api/webdav'
import { useSync } from '../composables/useSync'
import { useReaderStore, themePresets } from '../stores/reader'
import { readerFontOptions } from '../utils/readerFonts'
import { useUserStore } from '../stores/user'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const readerStore = useReaderStore()
const { connected: syncConnected } = useSync()

const settingPanels = new Set(['account', 'backup', 'cache', 'webdav', 'reader', 'replace', 'rss', 'admin'])
const activeTab = ref(settingPanels.has(String(route.query.panel || '')) ? String(route.query.panel) : 'account')
const checking = ref(false)
const backupLoading = ref(false)
const backupListLoading = ref(false)
const restoreLoading = ref(false)
const backups = ref([])
const users = ref([])
const usersLoading = ref(false)
const cleanupLoading = ref(false)
const webdavPath = ref('')
const webdavItems = ref([])
const webdavSelection = ref([])
const webdavLoading = ref(false)
const webdavUploading = ref(false)
const webdavRestoring = ref('')
const webdavImporting = ref(false)
const webdavImportResultDialog = ref(false)
const webdavImportResults = ref([])
const cacheStats = ref({})
const cacheLoading = ref(false)
const cacheClearing = ref(false)
const replaceRules = ref([])
const replaceRulesLoading = ref(false)
const replaceRuleDialog = ref(false)
const replaceRuleSaving = ref(false)
const replaceRuleTesting = ref(false)
const editingReplaceRuleId = ref(null)
const replaceRuleDraft = ref({ name: '', pattern: '', replacement: '', enabled: true })
const replaceRuleTestText = ref('广告123\n正文内容')
const replaceRuleTestResult = ref(null)
const readerBgUploading = ref(false)
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
const healthInfo = ref(null)
const windowWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
const coarsePointer = ref(typeof window === 'undefined' ? false : window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false)

const fontOptions = readerFontOptions

const readerModeModel = computed({
  get: () => readerStore.mode,
  set: value => readerStore.setMode(value),
})
const readerFontFamilyModel = computed({
  get: () => readerStore.fontFamily,
  set: value => readerStore.setFontFamily(value),
})
const readerBrightnessModel = computed({
  get: () => readerStore.brightness,
  set: value => readerStore.setBrightness(value),
})
const readerAutoReadSpeedModel = computed({
  get: () => readerStore.autoReadSpeed,
  set: value => readerStore.setAutoReadSpeed(value),
})
const readerFontSizeModel = computed({
  get: () => readerStore.fontSize,
  set: value => readerStore.setFontSize(value),
})
const readerFontWeightModel = computed({
  get: () => readerStore.fontWeight,
  set: value => readerStore.setFontWeight(value),
})
const readerLineHeightModel = computed({
  get: () => readerStore.lineHeight,
  set: value => readerStore.setLineHeight(value),
})
const readerParagraphSpaceModel = computed({
  get: () => readerStore.paragraphSpace,
  set: value => readerStore.setParagraphSpace(value),
})
const readerColumnWidthModel = computed({
  get: () => readerStore.columnWidth,
  set: value => readerStore.setColumnWidth(value),
})
const readerTTSRateModel = computed({
  get: () => readerStore.ttsRate,
  set: value => readerStore.setTTSRate(value),
})
const readerTTSPitchModel = computed({
  get: () => readerStore.ttsPitch,
  set: value => readerStore.setTTSPitch(value),
})

const webdavBreadcrumbs = computed(() => {
  if (!webdavPath.value) return []
  const parts = webdavPath.value.split('/').filter(Boolean)
  return parts.map((name, index) => ({ name, path: parts.slice(0, index + 1).join('/') }))
})
const webdavImportSelection = computed(() => webdavSelection.value.filter(row => row.importable))
const isMobileDialog = computed(() => windowWidth.value <= 860 || coarsePointer.value)

onMounted(() => {
  readerStore.normalizeSettings()
  window.addEventListener('resize', updateWindowWidth, { passive: true })
  loadBackups()
  loadWebDAV()
  loadCacheStats()
  loadHealthInfo().catch(() => {})
  loadReplaceRules()
  loadRSSSources()
  loadRSSArticles()
  if (userStore.profile?.role === 'admin') loadUsers()
})

onBeforeUnmount(() => window.removeEventListener('resize', updateWindowWidth))

function updateWindowWidth() {
  windowWidth.value = window.innerWidth
  coarsePointer.value = window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false
}

watch(
  () => route.query.panel,
  (panel) => {
    const value = String(panel || '')
    if (settingPanels.has(value)) activeTab.value = value
  },
)

async function checkHealth() {
  checking.value = true
  try {
    const data = await loadHealthInfo()
    const buildText = data.buildDate && data.buildDate !== 'unknown' ? `，构建 ${data.buildDate}` : ''
    ElMessage.success(`服务连接正常${buildText}`)
  } catch (err) {
    ElMessage.error(readError(err, '服务检查失败'))
  } finally {
    checking.value = false
  }
}

async function loadHealthInfo() {
  const { data } = await api.get('/health')
  healthInfo.value = data
  return data
}

function shortCommit(value) {
  if (!value || value === 'unknown') return '-'
  return String(value).slice(0, 12)
}

async function runBackup() {
  backupLoading.value = true
  try {
    const { data } = await triggerBackup()
    ElMessage.success(data?.path ? `备份已创建：${data.path}` : '备份已创建')
    await loadBackups()
  } catch (err) {
    ElMessage.error(readError(err, '备份失败'))
  } finally {
    backupLoading.value = false
  }
}

async function loadBackups() {
  backupListLoading.value = true
  try {
    const { data } = await listBackups()
    backups.value = data
  } catch (err) {
    ElMessage.error(readError(err, '加载备份失败'))
  } finally {
    backupListLoading.value = false
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
  } catch (err) {
    ElMessage.error(readError(err, '恢复失败'))
  } finally {
    restoreLoading.value = false
  }
}

async function download(row) {
  try {
    const resp = await downloadBackup(row.name)
    const a = document.createElement('a')
    a.href = URL.createObjectURL(new Blob([resp.data]))
    a.download = row.name
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (err) {
    ElMessage.error(readError(err, '下载失败'))
  }
}

async function loadCacheStats() {
  cacheLoading.value = true
  try {
    const { data } = await getCacheStats()
    cacheStats.value = data || {}
  } catch (err) {
    ElMessage.error(readError(err, '加载缓存统计失败'))
  } finally {
    cacheLoading.value = false
  }
}

async function clearSystemCache() {
  try {
    await ElMessageBox.confirm('确定清理全部章节缓存吗？清理后阅读时会重新加载章节内容。', '清理缓存', { type: 'warning' })
    cacheClearing.value = true
    const { data } = await clearCache()
    ElMessage.success(`已清理 ${data.clearedFiles || 0} 个文件，释放 ${formatSize(data.clearedSize || 0)}`)
    await loadCacheStats()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    cacheClearing.value = false
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

async function pickReaderBgImage(data) {
  const file = data.raw || data.file
  if (!file) return
  readerBgUploading.value = true
  try {
    const { data: result } = await uploadAsset({ file, type: 'background' })
    readerStore.setCustomBgImage(result.url)
    ElMessage.success('阅读背景图已上传')
  } catch (err) {
    ElMessage.error(readError(err, '上传背景图失败'))
  } finally {
    readerBgUploading.value = false
  }
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
    await updateReplaceRule(rule.id, rule)
    ElMessage.success('替换规则已更新')
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
    await ElMessageBox.confirm(`确定删除替换规则“${rule.name}”吗？`, '删除替换规则', { type: 'warning' })
    await deleteReplaceRule(rule.id)
    replaceRules.value = replaceRules.value.filter(item => item.id !== rule.id)
    ElMessage.success('替换规则已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除替换规则失败'))
  }
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

function rssArticleBody(article) {
  const text = article?.content || article?.summary || '无正文内容'
  return stripHTML(text)
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
    if (!webdavSelection.value.some(item => item.name === row.name)) {
      webdavSelection.value.push(row)
    }
    return
  }
  webdavSelection.value = webdavSelection.value.filter(item => item.name !== row.name)
}

function selectShownWebDAVFiles() {
  webdavSelection.value = webdavItems.value.filter(item => !item.isDir)
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
    const path = joinPath(webdavPath.value, row.name)
    const resp = await downloadWebDAV(path)
    const a = document.createElement('a')
    a.href = URL.createObjectURL(new Blob([resp.data]))
    a.download = row.name
    a.click()
    URL.revokeObjectURL(a.href)
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
    const current = joinPath(webdavPath.value, row.name)
    const target = joinPath(webdavPath.value, name)
    await renameWebDAV({ path: current, newPath: target })
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
  if (!paths.length) return
  await importWebDAVBooks(paths)
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
  } catch (err) {
    ElMessage.error(readError(err, '导入 WebDAV 文件失败'))
  } finally {
    webdavImporting.value = false
  }
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const { data } = await listUsers()
    users.value = data
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
    await ElMessageBox.confirm('确定清理不活跃用户吗？', '提示', { type: 'warning' })
    await cleanupInactiveUsers()
    ElMessage.success('清理完成')
    await loadUsers()
  } catch {
    // canceled or failed with message above from interceptor
  } finally {
    cleanupLoading.value = false
  }
}

function logout() {
  userStore.logout()
  router.push({ name: 'login' })
}

function limitText(value) {
  return value ? value : '不限制'
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

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.settings-page {
  display: grid;
  gap: 16px;
}

.settings-head,
.card-head,
.panel-actions,
.permission-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.settings-head {
  justify-content: space-between;
}

.eyebrow {
  margin: 0 0 4px;
  color: var(--app-primary);
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.settings-card {
  display: grid;
  align-content: start;
  gap: 14px;
  padding: 18px;
}

.card-head {
  color: var(--app-primary);
}

.card-head h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 17px;
}

.panel-text {
  margin: 0;
  color: var(--app-text-muted);
  line-height: 1.7;
}

.panel-actions {
  flex-wrap: wrap;
}

.info-list {
  display: grid;
  gap: 8px;
  margin: 0;
}

.info-list div {
  display: grid;
  grid-template-columns: 100px minmax(0, 1fr);
  gap: 12px;
}

.info-list dt {
  color: var(--app-text-muted);
}

.info-list dd {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.service-info {
  width: 100%;
  margin-top: 8px;
}

.backup-table {
  width: 100%;
}

.mobile-backup-list {
  display: none;
}

.mobile-backup-card {
  align-items: center;
  display: flex;
  gap: 10px;
  justify-content: space-between;
  padding: 12px;
}

.mobile-backup-card div {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.mobile-backup-card strong,
.mobile-backup-card span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-backup-card strong {
  color: var(--app-text);
  font-size: 14px;
}

.mobile-backup-card span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.webdav-breadcrumb button,
.file-name {
  padding: 0;
  color: var(--app-primary);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.file-name {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 8px;
  color: var(--app-text);
}

.file-name span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-file-list {
  display: none;
}

.mobile-file-select-actions {
  display: none;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  color: var(--app-text-muted);
  font-weight: 700;
}

.mobile-file-select-actions div {
  display: flex;
  gap: 4px;
}

.mobile-file-card {
  display: grid;
  gap: 9px;
  padding: 12px;
}

.mobile-file-card header,
.mobile-file-card footer,
.mobile-file-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-file-card header {
  justify-content: space-between;
}

.mobile-file-name {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
  font-weight: 700;
  text-align: left;
}

.mobile-file-name span,
.mobile-file-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-file-card p {
  margin: 0;
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-file-card footer {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.mobile-rule-list,
.mobile-user-list {
  display: none;
}

.mobile-rule-card,
.mobile-user-card {
  display: grid;
  gap: 9px;
  padding: 12px;
}

.mobile-rule-card header,
.mobile-rule-card footer,
.mobile-user-card header,
.mobile-permission-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-rule-card header,
.mobile-user-card header {
  justify-content: space-between;
}

.mobile-rule-card header > div,
.mobile-user-card header > div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.mobile-rule-card strong,
.mobile-rule-card span,
.mobile-rule-card p,
.mobile-user-card strong,
.mobile-user-card span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-rule-card strong,
.mobile-user-card strong {
  color: var(--app-text);
  font-size: 14px;
}

.mobile-rule-card span,
.mobile-rule-card p,
.mobile-user-card span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-rule-card p {
  margin: 0;
}

.mobile-rule-card footer,
.mobile-permission-row {
  justify-content: flex-end;
}

.reader-setting-list {
  display: grid;
  gap: 14px;
}

.reader-setting-list label {
  display: grid;
  gap: 6px;
}

.reader-setting-list span {
  color: var(--app-text-muted);
  font-size: 13px;
}

.replace-test-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: -4px 0 10px;
}

.msg-success {
  color: var(--app-success);
}

.msg-muted {
  color: var(--app-text-muted);
}

.replace-test-output {
  max-height: 180px;
  margin: 0 0 12px;
  overflow: auto;
  padding: 10px;
  color: var(--app-text);
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  white-space: pre-wrap;
}

.theme-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 10px;
}

.theme-choice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px;
  color: var(--app-text);
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  cursor: pointer;
  text-align: left;
}

.theme-choice.active,
.theme-choice:hover {
  border-color: var(--app-primary);
  background: var(--app-primary-soft);
}

.theme-swatch {
  width: 22px;
  height: 22px;
  border: 1px solid var(--app-border);
  border-radius: 50%;
}

.custom-swatch {
  background-image: linear-gradient(135deg, rgba(255,255,255,0.55), rgba(0,0,0,0.08));
}

.custom-theme-row {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--app-text-muted);
  font-size: 13px;
}

.rss-grid {
  grid-template-columns: minmax(280px, 0.85fr) minmax(0, 1.15fr);
}

.rss-source-list,
.rss-article-list {
  display: grid;
  gap: 10px;
}

.rss-source-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 10px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.rss-source-row.active {
  border-color: var(--app-primary);
  background: var(--app-primary-soft);
}

.rss-source-row button {
  min-width: 0;
  padding: 0;
  color: var(--app-text);
  text-align: left;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.rss-source-row strong,
.rss-source-row small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rss-source-row small,
.rss-article small {
  color: var(--app-text-muted);
}

.rss-article {
  display: grid;
  gap: 6px;
  padding: 12px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.rss-article.read {
  opacity: 0.68;
}

.rss-article-line {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.rss-article-title {
  padding: 0;
  color: var(--app-primary-strong);
  font-weight: 800;
  text-align: left;
  text-decoration: none;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.rss-article p {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--app-text-muted);
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.rss-reader {
  display: grid;
  gap: 12px;
}

.rss-reader h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 24px;
  line-height: 1.35;
}

.rss-reader small {
  color: var(--app-text-muted);
}

.rss-reader p {
  max-height: min(62vh, 680px);
  margin: 0;
  overflow: auto;
  color: var(--app-text);
  font-size: 16px;
  line-height: 1.85;
  white-space: pre-wrap;
}

.result-list {
  display: grid;
  gap: 10px;
}

.result-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 10px;
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.result-row span,
.result-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.result-row small {
  color: var(--app-text-muted);
}

.permission-row {
  flex-wrap: wrap;
}

code {
  padding: 2px 5px;
  color: var(--app-primary-strong);
  background: var(--app-primary-soft);
  border-radius: 4px;
}

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
  .settings-head,
  .settings-grid {
    display: grid;
    grid-template-columns: 1fr;
  }

  .desktop-webdav-table {
    display: none;
  }

  .desktop-backup-table {
    display: none;
  }

  .desktop-replace-table,
  .desktop-user-table {
    display: none;
  }

  .mobile-backup-list {
    display: grid;
    gap: 10px;
  }

  .mobile-rule-list,
  .mobile-user-list {
    display: grid;
    gap: 10px;
  }

  .mobile-file-list {
    display: grid;
    gap: 10px;
  }

  .mobile-file-select-actions {
    display: flex;
  }

  .rss-reader-dialog :deep(.el-dialog) {
    width: 94vw !important;
    margin-top: 3vh;
  }

  .rss-reader p {
    max-height: 70vh;
  }
}
</style>
