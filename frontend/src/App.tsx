import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { ThemeModeProvider } from '@/contexts/ThemeModeContext';
import { AppDataProvider } from '@/contexts/AppDataContext';
import { NotificationProvider } from '@/hooks/useNotify';
import { appRoutes } from '@/routes/appRoutes';

const router = createBrowserRouter(appRoutes);

export default function App() {
    return (
        <ThemeModeProvider>
            <NotificationProvider>
                <AppDataProvider>
                    <RouterProvider router={router} />
                </AppDataProvider>
            </NotificationProvider>
        </ThemeModeProvider>
    );
}
