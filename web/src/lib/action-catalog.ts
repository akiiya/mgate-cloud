import {
  Wifi,
  WifiOff,
  Power,
  PowerOff,
  Search,
  List,
  Gauge,
  ShieldCheck,
  ShieldOff,
  Network,
  Stethoscope,
  Activity,
  Radio,
  type LucideIcon,
} from 'lucide-react'

/**
 * 动作产品化目录。
 *
 * 这是「工程动作名 → 产品体验」的唯一映射层：把后端白名单 action（如 `ap.status`、
 * `tproxy.use`）翻译成普通用户看得懂的标题、说明、图标、表单与风险级别。
 * 页面只消费这里的产品语义，默认不直接展示裸 action 名（裸名仅在「高级详情」里出现）。
 *
 * 注意：这里只是展示层映射，真正的白名单与参数校验在后端
 * `internal/command/allowlist.go`，前后端各自维护、互不替代。
 */

/** 风险级别：safe 为只读/查询，caution 为会改变设备状态。 */
export type RiskLevel = 'safe' | 'caution'

/** 产品分类。 */
export type ActionCategory = 'hotspot' | 'network' | 'proxy' | 'gateway' | 'diagnostic'

/** 参数化动作的表单字段（仅 wlan.connect / tproxy.use 使用）。 */
export interface ActionField {
  /** 提交到后端的参数键名。 */
  name: string
  label: string
  placeholder?: string
  required?: boolean
  help?: string
}

export interface ActionCatalogEntry {
  /** 工程动作名（与后端白名单一致），仅在高级详情展示。 */
  action: string
  /** 面向用户的标题。 */
  title: string
  /** 通俗说明：这个操作会做什么。 */
  description: string
  category: ActionCategory
  riskLevel: RiskLevel
  icon: LucideIcon
  /** 参数化动作的表单定义；无参动作省略。 */
  fields?: ActionField[]
  /** 成功后的简短文案。 */
  successHint: string
  /** 失败后的简短文案。 */
  failureHint: string
  /** 设备离线时的提示（命令会进入队列）。 */
  offlineHint: string
}

export interface CategoryMeta {
  key: ActionCategory
  label: string
  description: string
  icon: LucideIcon
}

/** 分类元信息：用于把操作分组陈列。 */
export const categoryMeta: Record<ActionCategory, CategoryMeta> = {
  hotspot: { key: 'hotspot', label: '热点', description: '设备自身 WiFi 热点的开关与状态', icon: Radio },
  network: { key: 'network', label: '上网 / WiFi', description: '连接上游 WiFi、扫描与查看网络', icon: Wifi },
  proxy: { key: 'proxy', label: '加速 / 代理', description: '透明代理的开关与线路切换', icon: Gauge },
  gateway: { key: 'gateway', label: '网关', description: '网关服务的启停与状态', icon: Network },
  diagnostic: { key: 'diagnostic', label: '诊断', description: '设备自检与健康检查', icon: Stethoscope },
}

const offlineQueued = '设备当前未在线，操作会进入队列，待设备重新连接或 Pull 后执行。'

/**
 * 目录主体：键为工程 action 名。新增动作时在此登记产品语义，
 * 务必与后端 allowlist 同步。
 */
export const actionCatalog: Record<string, ActionCatalogEntry> = {
  'ap.status': {
    action: 'ap.status',
    title: '查看热点状态',
    description: '读取设备热点当前是否开启及基本信息。',
    category: 'hotspot',
    riskLevel: 'safe',
    icon: Activity,
    successHint: '已获取热点状态。',
    failureHint: '获取热点状态失败。',
    offlineHint: offlineQueued,
  },
  'ap.start': {
    action: 'ap.start',
    title: '开启热点',
    description: '打开设备自身的 WiFi 热点，供终端连接。',
    category: 'hotspot',
    riskLevel: 'caution',
    icon: Power,
    successHint: '热点已开启。',
    failureHint: '开启热点失败。',
    offlineHint: offlineQueued,
  },
  'ap.stop': {
    action: 'ap.stop',
    title: '关闭热点',
    description: '关闭设备自身的 WiFi 热点。',
    category: 'hotspot',
    riskLevel: 'caution',
    icon: PowerOff,
    successHint: '热点已关闭。',
    failureHint: '关闭热点失败。',
    offlineHint: offlineQueued,
  },
  'wlan.scan': {
    action: 'wlan.scan',
    title: '扫描可用 WiFi',
    description: '让设备扫描周围可连接的上游 WiFi。',
    category: 'network',
    riskLevel: 'safe',
    icon: Search,
    successHint: '扫描完成。',
    failureHint: '扫描失败。',
    offlineHint: offlineQueued,
  },
  'wlan.list': {
    action: 'wlan.list',
    title: '查看 WiFi 列表',
    description: '查看设备已保存或可见的上游 WiFi。',
    category: 'network',
    riskLevel: 'safe',
    icon: List,
    successHint: '已获取 WiFi 列表。',
    failureHint: '获取 WiFi 列表失败。',
    offlineHint: offlineQueued,
  },
  'wlan.connect': {
    action: 'wlan.connect',
    title: '连接 WiFi',
    description: '让设备连接指定的上游 WiFi 网络。',
    category: 'network',
    riskLevel: 'caution',
    icon: Wifi,
    fields: [
      {
        name: 'ssid',
        label: 'WiFi 名称（SSID）',
        placeholder: '例如：Home-5G',
        required: true,
        help: '出于安全，控制面不接收 WiFi 密码；密码由设备本地保存或预置。',
      },
    ],
    successHint: '已下发连接请求。',
    failureHint: '连接 WiFi 失败。',
    offlineHint: offlineQueued,
  },
  'tproxy.enable': {
    action: 'tproxy.enable',
    title: '开启加速',
    description: '开启设备的透明代理（加速）。',
    category: 'proxy',
    riskLevel: 'caution',
    icon: ShieldCheck,
    successHint: '加速已开启。',
    failureHint: '开启加速失败。',
    offlineHint: offlineQueued,
  },
  'tproxy.disable': {
    action: 'tproxy.disable',
    title: '关闭加速',
    description: '关闭设备的透明代理（加速）。',
    category: 'proxy',
    riskLevel: 'caution',
    icon: ShieldOff,
    successHint: '加速已关闭。',
    failureHint: '关闭加速失败。',
    offlineHint: offlineQueued,
  },
  'tproxy.use': {
    action: 'tproxy.use',
    title: '切换线路节点',
    description: '切换加速使用的线路 / 节点。',
    category: 'proxy',
    riskLevel: 'caution',
    icon: Gauge,
    fields: [{ name: 'node', label: '节点名称', placeholder: '例如：HK-01', required: true }],
    successHint: '已下发节点切换。',
    failureHint: '切换节点失败。',
    offlineHint: offlineQueued,
  },
  'gateway.status': {
    action: 'gateway.status',
    title: '查看网关状态',
    description: '读取网关服务的运行状态。',
    category: 'gateway',
    riskLevel: 'safe',
    icon: Activity,
    successHint: '已获取网关状态。',
    failureHint: '获取网关状态失败。',
    offlineHint: offlineQueued,
  },
  'gateway.start': {
    action: 'gateway.start',
    title: '启动网关',
    description: '启动设备的网关服务。',
    category: 'gateway',
    riskLevel: 'caution',
    icon: Power,
    successHint: '网关已启动。',
    failureHint: '启动网关失败。',
    offlineHint: offlineQueued,
  },
  'gateway.stop': {
    action: 'gateway.stop',
    title: '停止网关',
    description: '停止设备的网关服务。',
    category: 'gateway',
    riskLevel: 'caution',
    icon: PowerOff,
    successHint: '网关已停止。',
    failureHint: '停止网关失败。',
    offlineHint: offlineQueued,
  },
  'doctor.full': {
    action: 'doctor.full',
    title: '一键体检',
    description: '运行设备全量自检，输出健康诊断。',
    category: 'diagnostic',
    riskLevel: 'safe',
    icon: Stethoscope,
    successHint: '体检完成。',
    failureHint: '体检失败。',
    offlineHint: offlineQueued,
  },
}

/** 控制台「设备操作」陈列顺序：按分类与常用度排列。 */
export const actionOrder: string[] = [
  'ap.status',
  'ap.start',
  'ap.stop',
  'wlan.scan',
  'wlan.list',
  'wlan.connect',
  'tproxy.enable',
  'tproxy.disable',
  'tproxy.use',
  'gateway.status',
  'gateway.start',
  'gateway.stop',
  'doctor.full',
]

/** 取某个 action 的产品条目；未知 action 回退为通用条目（仍隐藏裸名细节）。 */
export function getAction(action: string): ActionCatalogEntry {
  return (
    actionCatalog[action] ?? {
      action,
      title: action,
      description: '设备操作。',
      category: 'diagnostic',
      riskLevel: 'caution',
      icon: WifiOff,
      successHint: '操作完成。',
      failureHint: '操作失败。',
      offlineHint: offlineQueued,
    }
  )
}

/** 友好标题：用户可读的动作名（找不到时回退裸名）。 */
export function actionTitle(action: string): string {
  return actionCatalog[action]?.title ?? action
}

/** 按分类分组的动作（保持 actionOrder 的相对顺序）。 */
export function actionsByCategory(): { category: CategoryMeta; entries: ActionCatalogEntry[] }[] {
  const order: ActionCategory[] = ['hotspot', 'network', 'proxy', 'gateway', 'diagnostic']
  return order.map((key) => ({
    category: categoryMeta[key],
    entries: actionOrder.map(getAction).filter((e) => e.category === key),
  }))
}
