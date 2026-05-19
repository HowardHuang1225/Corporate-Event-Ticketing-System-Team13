export interface User {
  id: string
  employee_id: string
  name: string
  email: string
  department: string
  region: string
  role: 'employee' | 'event_manager' | 'hr'
}