<template>
  <main class="p-6 bg-gray-100 min-h-screen">
    <h1 class="text-3xl font-bold text-blue-600 mb-6">📊 文档点击排行榜</h1>

    <!-- 文档点击列表 -->
    <section class="mb-8">
      <h2 class="text-xl font-semibold mb-2">📁 可点击文档</h2>
      <div class="flex flex-wrap gap-4">
        <button
            v-for="doc in documents"
            :key="doc.id"
            @click="clickDoc(doc.id)"
            class="px-4 py-2 bg-blue-500 hover:bg-blue-600 text-white rounded-lg shadow"
        >
          {{ doc.title }}
        </button>
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

// 可点击的文档（固定）
const documents = [
  { id: 'a', title: '文档 A' },
  { id: 'b', title: '文档 B' },
  { id: 'c', title: '文档 C' },
]

const totalRank = ref([])
const recentRank = ref([])

async function loadRankings() {
  try {
    const [totalRes, recentRes] = await Promise.all([
      fetch('http://localhost:8080/rank/total').then(res => res.json()),
      fetch('http://localhost:8080/rank/recent').then(res => res.json()),
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
    await fetch('http://localhost:8080/click', {
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
  const doc = documents.find(d => d.id === docID)
  return doc ? doc.title : docID
}

onMounted(() => {
  loadRankings()

  const source = new EventSource('http://localhost:8080/events')
  source.addEventListener('ranking_update', (event) => {
    try {
      const data = JSON.parse(event.data)
      totalRank.value = data.total_rank || []
      recentRank.value = data.recent_rank || []
    } catch (err) {
      console.error('解析 SSE 数据失败:', err)
    }
  })
  source.onerror = (err) => {
    console.warn('SSE 连接失败或断开', err)
  }
})

</script>
