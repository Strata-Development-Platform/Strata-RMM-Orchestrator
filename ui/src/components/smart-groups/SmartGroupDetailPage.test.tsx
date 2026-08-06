import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import SmartGroupDetailPage from './SmartGroupDetailPage';
import SmartGroupExpressionBuilder from './SmartGroupExpressionBuilder';
import type { SmartGroupExpression } from './SmartGroupTypes';

vi.mock('@/components/shared/Toast', () => ({
  useToast: () => ({
    showToast: vi.fn(),
  }),
}));

describe('SmartGroupDetailPage', () => {
  it('renders group name and description', () => {
    render(<SmartGroupDetailPage groupId="group-001" />);
    expect(screen.getByText('Linux Servers')).toBeInTheDocument();
  });

  it('displays member count', () => {
    render(<SmartGroupDetailPage groupId="group-001" />);
    expect(screen.getByText('42')).toBeInTheDocument();
  });

  it('shows group type as Smart', () => {
    render(<SmartGroupDetailPage groupId="group-001" />);
    expect(screen.getByText('Smart')).toBeInTheDocument();
  });

  it('renders tab navigation buttons', () => {
    render(<SmartGroupDetailPage groupId="group-001" />);
    expect(screen.getByRole('button', { name: /Overview/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Members/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Bindings/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Expression/i })).toBeInTheDocument();
  });

  it('switches to members tab', () => {
    render(<SmartGroupDetailPage groupId="group-001" />);
    fireEvent.click(screen.getByRole('button', { name: /Members/i }));
    expect(screen.getByText(/web-server-01/)).toBeInTheDocument();
  });

  it('switches to expression tab', () => {
    render(<SmartGroupDetailPage groupId="group-001" />);
    fireEvent.click(screen.getByRole('button', { name: /Expression/i }));
    expect(screen.getByText('Expression Builder')).toBeInTheDocument();
  });
});

describe('SmartGroupExpressionBuilder', () => {
  it('renders with default empty expression', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    expect(screen.getByText('Expression Builder')).toBeInTheDocument();
  });

  it('renders with initial expression', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [
        { field: 'os', op: 'startswith', value: 'Linux' },
      ],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    expect(screen.getByText('Expression Builder')).toBeInTheDocument();
  });

  it('allows adding a new filter', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    fireEvent.click(screen.getByText('Add Filter'));
    expect(onChange).toHaveBeenCalled();
  });

  it('allows removing a filter', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [
        { field: 'os', op: 'eq', value: 'Linux' },
        { field: 'platform', op: 'eq', value: 'linux' },
      ],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    const removeButtons = screen.getAllByTitle('Remove filter');
    if (removeButtons.length > 0) {
      fireEvent.click(removeButtons[0]);
      expect(onChange).toHaveBeenCalled();
    }
  });

  it('switches to JSON view mode', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    const jsonBtn = screen.getByText('JSON');
    expect(jsonBtn).toBeInTheDocument();
    fireEvent.click(jsonBtn);
    expect(screen.getByText('JSON')).toHaveClass('bg-blue-600');
  });

  it('renders field and operator selectors when filters exist', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [
        { field: 'os', op: 'eq', value: 'Linux' },
      ],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    const selects = screen.getAllByRole('combobox');
    expect(selects.length).toBeGreaterThanOrEqual(2);
  });

  it('logic operator changes expression condition', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    fireEvent.change(select, { target: { value: 'OR' } });
    expect(onChange).toHaveBeenCalled();
  });

  it('renders add nested group button', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    expect(screen.getByText('Nested Group')).toBeInTheDocument();
  });
});
