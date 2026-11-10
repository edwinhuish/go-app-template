/** 接口前缀。vite.config.ts 的 server.proxy 负责把它代理到后端。 */
export const API_BASE = '/api/v1'

/**
 * 令牌在 localStorage 里的键。
 *
 * 带上应用前缀，避免同一域名下多个应用互相覆盖 localStorage。
 * 换成自己的项目名时，这里和 localStorage 里的历史数据要一起考虑。
 */
export const TOKEN_KEY = 'app_token'

/** 默认分页大小，与后端 dto.DefaultPageSize 保持一致。 */
export const DEFAULT_PAGE_SIZE = 20
