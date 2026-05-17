import { createContext, useContext } from 'react'

export interface User {
  id: string
  employee_id: string
  name: string
  email: string
  department: string
  region: string
  role: 'employee' | 'event_manager' | 'hr'
}

export interface AuthContextType {
  user: User | null
  login: (employeeId: string, password: string) => Promise<void>
  logout: () => void
  isLoading: boolean
}

export const AuthContext = createContext<AuthContextType | null>(null)

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be inside AuthProvider')
  return ctx
}