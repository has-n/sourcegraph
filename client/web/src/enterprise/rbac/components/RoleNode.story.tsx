import { DecoratorFn, Meta, Story } from '@storybook/react'
import { noop } from 'lodash'
import { MATCH_ANY_PARAMETERS, WildcardMockLink } from 'wildcard-mock-link'

import { getDocumentNode } from '@sourcegraph/http-client'
import { MockedTestProvider } from '@sourcegraph/shared/src/testing/apollo'

import { WebStory } from '../../../components/WebStory'
import { DELETE_ROLE } from '../backend'
import { mockRoles, mockPermissionsMap } from '../mock'

import { RoleNode } from './RoleNode'

const decorator: DecoratorFn = story => <div className="p-3 container list-unstyled">{story()}</div>

const config: Meta = {
    title: 'web/src/site-admin/rbac/RoleNode',
    decorators: [decorator],
}

export default config

const mocks = new WildcardMockLink([
    {
        request: {
            query: getDocumentNode(DELETE_ROLE),
            variables: MATCH_ANY_PARAMETERS,
        },
        result: { data: { deleteRole: { alwaysNil: null } } },
        nMatches: Number.POSITIVE_INFINITY,
    },
])

const [systemRole, nonSystemRole] = mockRoles.roles.nodes

export const SystemRole: Story = () => (
    <WebStory>
        {() => (
            <MockedTestProvider link={mocks}>
                <RoleNode allPermissions={mockPermissionsMap} node={systemRole} afterDelete={noop} />
            </MockedTestProvider>
        )}
    </WebStory>
)

SystemRole.storyName = 'System role'

export const NonSystemRole: Story = () => (
    <WebStory>
        {() => (
            <MockedTestProvider link={mocks}>
                <RoleNode allPermissions={mockPermissionsMap} node={nonSystemRole} afterDelete={noop} />
            </MockedTestProvider>
        )}
    </WebStory>
)

NonSystemRole.storyName = 'Non-system role'
