import { apiClient } from '@/api/client'

export type LocalizedDocText = { zh: string; en: string }

export interface DocArticle {
  id: string
  slug: string
  published: boolean
  title: LocalizedDocText
  content: LocalizedDocText
}

export async function getPublishedDocs(): Promise<DocArticle[]> {
  const { data } = await apiClient.get<DocArticle[]>('/docs')
  return data
}

export async function getAdminDocs(): Promise<DocArticle[]> {
  const { data } = await apiClient.get<DocArticle[]>('/admin/docs')
  return data
}

export async function saveAdminDocs(docs: DocArticle[]): Promise<DocArticle[]> {
  const { data } = await apiClient.put<DocArticle[]>('/admin/docs', docs)
  return data
}
