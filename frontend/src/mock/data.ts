export const MOCK_SONGS = [
  {
    bvid: 'BV1uT4y1P7CX',
    title: '晴天',
    artist: '周杰伦',
    cover: '//i0.hdslb.com/bfs/archive/8d9a7c6f5e3b2a1c0d4e5f6g7h8i9j0.jpg',
    duration: '4:29',
    duration_sec: 269,
    play_count: 12000000,
  },
  {
    bvid: 'BV1GJ411x7h7',
    title: '稻香',
    artist: '周杰伦',
    cover: '//i0.hdslb.com/bfs/archive/a1b2c3d4e5f6g7h8i9j0k1l2m3n4.jpg',
    duration: '3:42',
    duration_sec: 222,
    play_count: 8500000,
  },
  {
    bvid: 'BV1xx411c7mu',
    title: '七里香',
    artist: '周杰伦',
    cover: '//i0.hdslb.com/bfs/archive/z1y2x3w4v5u6t7r8s9q0w1e2r3t4.jpg',
    duration: '5:01',
    duration_sec: 301,
    play_count: 15000000,
  },
  {
    bvid: 'BV1yW411v7xD',
    title: '青花瓷',
    artist: '周杰伦',
    cover: '//i0.hdslb.com/bfs/archive/b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6.jpg',
    duration: '3:59',
    duration_sec: 239,
    play_count: 20000000,
  },
  {
    bvid: 'BV1aE411o7qP',
    title: '夜曲',
    artist: '周杰伦',
    cover: '//i0.hdslb.com/bfs/archive/c4d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9.jpg',
    duration: '4:15',
    duration_sec: 255,
    play_count: 9800000,
  },
  {
    bvid: 'BV1U1o9B6Eby',
    title: '起风了',
    artist: '买辣椒也用券',
    cover: '//i0.hdslb.com/bfs/archive/d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0.jpg',
    duration: '5:20',
    duration_sec: 320,
    play_count: 50000000,
  },
  {
    bvid: 'BV1bK4y1N7qR',
    title: '孤勇者',
    artist: '陈奕迅',
    cover: '//i0.hdslb.com/bfs/archive/e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t1.jpg',
    duration: '4:16',
    duration_sec: 256,
    play_count: 80000000,
  },
  {
    bvid: 'BV1iJ411x7hQ',
    title: '平凡之路',
    artist: '朴树',
    cover: '//i0.hdslb.com/bfs/archive/f7g8h9i0j1k2l3m4n5o6p7q8r9s0t1u2.jpg',
    duration: '4:46',
    duration_sec: 286,
    play_count: 65000000,
  },
]

export const MOCK_FAVLISTS = [
  { mid: 2320857281, name: '我喜欢的音乐', count: 128 },
  { mid: 1983427561, name: '华语经典', count: 86 },
  { mid: 1765432109, name: '深夜电台', count: 45 },
  { mid: 2098765432, name: '运动歌单', count: 32 },
]

export const MOCK_FAV_MEDIA = [
  {
    id: 1, bvid: 'BV1uT4y1P7CX', title: '晴天', cover: '//i0.hdslb.com/bfs/archive/8d9a7c6f5e3b2a1c0d4e5f6g7h8i9j0.jpg',
    duration: 269, artist: '周杰伦',
  },
  {
    id: 2, bvid: 'BV1GJ411x7h7', title: '稻香', cover: '//i0.hdslb.com/bfs/archive/a1b2c3d4e5f6g7h8i9j0k1l2m3n4.jpg',
    duration: 222, artist: '周杰伦',
  },
  {
    id: 3, bvid: 'BV1xx411c7mu', title: '七里香', cover: '//i0.hdslb.com/bfs/archive/z1y2x3w4v5u6t7r8s9q0w1e2r3t4.jpg',
    duration: 301, artist: '周杰伦',
  },
  {
    id: 4, bvid: 'BV1yW411v7xD', title: '青花瓷', cover: '//i0.hdslb.com/bfs/archive/b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6.jpg',
    duration: 239, artist: '周杰伦',
  },
  {
    id: 5, bvid: 'BV1aE411o7qP', title: '夜曲', cover: '//i0.hdslb.com/bfs/archive/c4d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9.jpg',
    duration: 255, artist: '周杰伦',
  },
]

export const MOCK_FAV_INFO = {
  id: 2320857281,
  title: '我喜欢的音乐',
  cover: '//i0.hdslb.com/bfs/archive/x1y2z3w4v5u6t7r8s9q0w1e2r3t4u5v6.jpg',
  media_count: 128,
  intro: '收藏的喜欢的歌曲',
  upper: { mid: 12345678, name: 'B站用户', face: '//i0.hdslb.com/bfs/face/abc123def456.jpg' },
}

export const MOCK_DLNA_DEVICES = [
  { name: 'Samsung TV 55"', udn: 'uuid:samsung-tv-001', ip: '192.168.1.100', port: 8080, device_type: 'MediaRenderer' },
  { name: 'Xiaomi TV 65"', udn: 'uuid:xiaomi-tv-002', ip: '192.168.1.101', port: 49152, device_type: 'MediaRenderer' },
  { name: 'PS5 Pro', udn: 'uuid:ps5-003', ip: '192.168.1.102', port: 9000, device_type: 'MediaRenderer' },
]

export const MOCK_SPEAKER_DEVICES = [
  { did: 'speaker_001', name: '小爱音箱Pro', hardware: 'L06A', is_online: true },
  { did: 'speaker_002', name: '小爱音箱Play', hardware: 'L05B', is_online: true },
  { did: 'speaker_003', name: '小爱音箱卧室', hardware: 'LX04', is_online: false },
]

export const MOCK_SNIFF_RESULT = {
  title: '某部电视剧 第一季',
  sniff_method: 'yt-dlp',
  episodes: [
    { index: 1, title: '第1集 双线突袭', url: 'https://example.com/ep1.mp4', duration: 2720, thumbnail: '' },
    { index: 2, title: '第2集 初露锋芒', url: 'https://example.com/ep2.mp4', duration: 2655, thumbnail: '' },
    { index: 3, title: '第3集 暗流涌动', url: 'https://example.com/ep3.m3u8', duration: 2638, thumbnail: '' },
    { index: 4, title: '第4集 风云突变', url: 'https://example.com/ep4.mp4', duration: 2701, thumbnail: '' },
    { index: 5, title: '第5集 绝地反击', url: 'https://example.com/ep5.mp4', duration: 2688, thumbnail: '' },
    { index: 6, title: '第6集 黎明之前', url: 'https://example.com/ep6.mp4', duration: 2745, thumbnail: '' },
    { index: 7, title: '第7集 深入虎穴', url: 'https://example.com/ep7.m3u8', duration: 2699, thumbnail: '' },
    { index: 8, title: '第8集 最终决战', url: 'https://example.com/ep8.mp4', duration: 2800, thumbnail: '' },
  ],
}

const COVER_BASE = 'https://picsum.photos/seed'
