import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * cn 合并 className：clsx 处理条件类名，tailwind-merge 解决 Tailwind 类冲突
 * （如同时出现 px-2 与 px-4 时保留后者）。这是 shadcn 风格组件的标准工具。
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
