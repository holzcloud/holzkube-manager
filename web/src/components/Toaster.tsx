import { toast } from 'sonner'
import { Toaster as SonnerToaster } from '@/components/ui/sonner'

/**
 * The application's toast surface, with the four levels the UI actually uses:
 * success, information, warning and error.
 *
 * Re-exported through this module rather than used directly from `sonner` so
 * there is one place that decides position, duration and the level vocabulary.
 */

export function Toaster() {
  return <SonnerToaster position="bottom-right" closeButton richColors={false} duration={6000} />
}

export const notify = {
  success: (message: string, description?: string) => toast.success(message, { description }),
  info: (message: string, description?: string) => toast.info(message, { description }),
  warning: (message: string, description?: string) => toast.warning(message, { description }),
  error: (message: string, description?: string) => toast.error(message, { description }),
}
