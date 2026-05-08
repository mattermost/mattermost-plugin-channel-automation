import React from 'react';
import {Link, useRouteMatch} from 'react-router-dom';

interface NavItem {
    id: string;
    label: string;
    icon: React.ReactNode;
    path: string;
}

const AutomationsIcon = () => (
    <svg
        width='16'
        height='16'
        viewBox='0 0 24 24'
        fill='none'
        stroke='currentColor'
        strokeWidth='2'
        strokeLinecap='round'
        strokeLinejoin='round'
    >
        <polyline points='22 12 18 12 15 21 9 3 6 12 2 12'/>
    </svg>
);

const NAV_ITEMS: NavItem[] = [
    {id: 'automations', label: 'Automations', icon: <AutomationsIcon/>, path: '/automations'},
];

interface SidebarProps {
    baseUrl: string;
}

const NavLink: React.FC<{item: NavItem; baseUrl: string}> = ({item, baseUrl}) => {
    const match = useRouteMatch(`${baseUrl}${item.path}`);
    return (
        <Link
            className={`channel-automation-sidebar__nav-item${match ? ' channel-automation-sidebar__nav-item--active' : ''}`}
            to={`${baseUrl}${item.path}`}
        >
            {item.icon}
            {item.label}
        </Link>
    );
};

const Sidebar: React.FC<SidebarProps> = ({baseUrl}) => {
    return (
        <nav className='channel-automation-sidebar'>
            {NAV_ITEMS.map((item) => (
                <NavLink
                    key={item.id}
                    item={item}
                    baseUrl={baseUrl}
                />
            ))}
        </nav>
    );
};

export default Sidebar;
