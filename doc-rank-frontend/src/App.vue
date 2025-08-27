<template>
  <main class="p-6 bg-gray-100 min-h-screen">
    <h1 class="text-3xl font-bold text-blue-600 mb-6">📊 文档点击排行榜</h1>

    <!-- 文档新增/编辑表单 -->
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <input v-model="newDoc.id" placeholder="ID" :disabled="editingId !== null"
        class="border px-2 py-1 rounded w-28" />
      <input v-model="newDoc.title" placeholder="标题"
        class="border px-2 py-1 rounded w-40" />
      <input v-model="newDoc.url" placeholder="URL（可选）"
        class="border px-2 py-1 rounded w-48" />

      <button
          @click="saveDoc"
          class="px-3 py-1 bg-green-500 hover:bg-green-600 text-white rounded"
      >
        {{ editingId ? '💾 保存修改' : '➕ 添加文档' }}
      </button>

      <button
          v-if="editingId"
          @click="cancelEdit"
          class="px-3 py-1 bg-gray-300 hover:bg-gray-400 text-black rounded"
      >
        取消编辑
      </button>
    </div>

    <!-- 文档点击列表 -->
    <section class="mb-8">
      <h2 class="text-xl font-semibold mb-2">📁 已添加文档</h2>
      <div class="flex flex-wrap gap-4">
        <div v-for="doc in documents" :key="doc.id" class="flex items-center gap-2">
          <button
              @click="clickDoc(doc.id)"
              class="px-4 py-2 bg-blue-500 hover:bg-blue-600 text-white rounded shadow"
          >
            {{ doc.title }}
          </button>

          <button
              @click="editDoc(doc)"
              class="text-yellow-500 hover:text-yellow-700"
              title="编辑"
          >✏️</button>

          <button
              @click="deleteDoc(doc.id)"
              class="text-red-500 hover:text-red-700"
              title="删除"
          >🗑️</button>
        </div>
      </div>
    </section>

    <!-- 排行榜 -->
    <section>
      <div class="flex justify-between items-center mb-2">
        <h2 class="text-xl font-semibold">🏆 实时排行榜</h2>
        <button
            @click="loadRankings"
            class="text-sm px-3 py-1 bg-gray-200 hover:bg-gray-300 rounded shadow"
        >
          🔄 手动刷新
        </button>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div class="bg-white rounded-lg p-4 shadow">
          <h3 class="font-bold text-lg mb-2">🔢 总点击排行榜</h3>
          <ol>
            <li v-for="(item, index) in totalRank" :key="item.doc_id" class="mb-1">
              <span class="font-semibold">{{ index + 1 }}. {{ getTitle(item.doc_id) }}</span> - {{ item.clicks }} 次
            </li>
          </ol>
        </div>

        <div class="bg-white rounded-lg p-4 shadow">
          <h3 class="font-bold text-lg mb-2">⏱️ 最近 10 分钟排行榜</h3>
          <ol>
            <li v-for="(item, index) in recentRank" :key="item.doc_id" class="mb-1">
              <span class="font-semibold">{{ index + 1 }}. {{ getTitle(item.doc_id) }}</span> - {{ item.clicks }} 次
            </li>
          </ol>
        </div>
      </div>
    </section>
  </main>
</template>

<script setup>
import { ref, onMounted } from 'vue'

// 使用实时查询的文档列表
const documents = ref([])
const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

async function loadDocuments() {
  try {
    const res = await fetch(`${apiBaseUrl}/docs`)
    const data = await res.json()
    documents.value = data.documents || []
  } catch (err) {
    console.error('加载文档失败:', err)
  }
}

const totalRank = ref([])
const recentRank = ref([])

async function loadRankings() {
  try {
    const [totalRes, recentRes] = await Promise.all([
      fetch(`${apiBaseUrl}/rank/total`).then(res => res.json()),
      fetch(`${apiBaseUrl}/rank/recent`).then(res => res.json()),
    ])
    totalRank.value = totalRes.rank || []
    recentRank.value = recentRes.rank || []
  } catch (err) {
    console.error('获取排行榜失败:', err)
  }
}

// 点击文档
async function clickDoc(docID) {
  try {
    await fetch(`${apiBaseUrl}/click`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ doc_id: docID }),
    })
  } catch (err) {
    console.error('点击失败:', err)
  }
}

// 获取文档标题
function getTitle(docID) {
  const doc = documents.value.find(d => d.id === docID)
  return doc ? doc.title : `新文档 (${docID})`
}

// 提供文件的增删改功能

// 增与改
const newDoc = ref({ id: '', title: '', url: '' })
const editingId = ref(null) // 用于编辑文档

async function saveDoc() {
  const body = { ...newDoc.value }

  if (!body.id || !body.title) {
    alert('ID 和标题不能为空')
    return
  }

  try {
    const res = await fetch(`${apiBaseUrl}/docs`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (res.ok) {
      newDoc.value = { id: '', title: '', url: '' }
      editingId.value = null
      await loadDocuments()
    }
  } catch (err) {
    console.error('保存失败:', err)
  }
}

function editDoc(doc) {
  newDoc.value = { ...doc }
  editingId.value = doc.id
}

function cancelEdit() {
  newDoc.value = { id: '', title: '', url: '' }
  editingId.value = null
}

// 删
async function deleteDoc(id) {
  if (!confirm(`确认删除文档 ${id} 吗？`)) return
  try {
    const res = await fetch(`${apiBaseUrl}/docs/${id}`, {
      method: 'DELETE',
    })
    if (res.ok) {
      await loadDocuments()
    }
  } catch (err) {
    console.error('删除失败:', err)
  }
}

onMounted(() => {
  loadDocuments()
  loadRankings()

  const source = new EventSource(`${apiBaseUrl}/events`)
  source.addEventListener('update', async (event) => {
    try {
      const data = JSON.parse(event.data)
      if (data.type === 'update_all') {
        await loadDocuments()
        await loadRankings()
      }
    } catch (err) {
      console.error('解析 SSE 数据失败:', err)
    }
  })
  source.onerror = (err) => {
    console.warn('SSE 连接失败或断开', err)
  }
})

</script>
