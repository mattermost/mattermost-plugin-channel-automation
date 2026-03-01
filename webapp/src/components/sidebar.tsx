import React from 'react';

interface NavItem {
    id: string;
    label: string;
    icon: React.ReactNode;
}

const FlowsIcon = () => (
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
    {id: 'flows', label: 'Flows', icon: <FlowsIcon/>},
];

interface SidebarProps {
    activeItem: string;
    onItemClick: (id: string) => void;
}

const Sidebar: React.FC<SidebarProps> = ({activeItem, onItemClick}) => {
    return (
        <nav className='channel-automation-sidebar'>
            {NAV_ITEMS.map((item) => (
                <button
                    key={item.id}
                    className={`channel-automation-sidebar__nav-item${activeItem === item.id ? ' channel-automation-sidebar__nav-item--active' : ''}`}
                    onClick={() => onItemClick(item.id)}
                >
                    {item.icon}
                    {item.label}
                </button>
            ))}
        </nav>
    );
};

export default Sidebar;
