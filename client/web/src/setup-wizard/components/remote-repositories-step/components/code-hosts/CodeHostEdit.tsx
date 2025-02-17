import { FC, ReactNode } from 'react'

import { useQuery } from '@apollo/client'
import { useNavigate, useParams } from 'react-router-dom'

import { useMutation } from '@sourcegraph/http-client'
import { ExternalServiceKind } from '@sourcegraph/shared/src/graphql-operations'
import { Alert, Button, ErrorAlert, H4, Link, LoadingSpinner } from '@sourcegraph/wildcard'

import { defaultExternalServices } from '../../../../../components/externalServices/externalServices'
import { LoaderButton } from '../../../../../components/LoaderButton'
import {
    AddExternalServiceInput,
    GetExternalServiceByIdResult,
    GetExternalServiceByIdVariables,
    UpdateRemoteCodeHostResult,
    UpdateRemoteCodeHostVariables,
} from '../../../../../graphql-operations'
import { GET_CODE_HOST_BY_ID, UPDATE_CODE_HOST } from '../../queries'

import { CodeHostConnectFormFields, CodeHostJSONForm, CodeHostJSONFormState } from './common'
import { GithubConnectView } from './github/GithubConnectView'

import styles from './CodeHostCreation.module.scss'

/**
 * Manually created type based on gql query schema, auto-generated tool can't infer
 * proper type for ExternalServices (because of problems with gql schema itself, node
 * type implementation problem that leads to have a massive union if when you use specific
 * type fragment)
 */
interface EditableCodeHost {
    __typename: 'ExternalService'
    id: string
    kind: ExternalServiceKind
    displayName: string
    config: string
}

interface CodeHostEditProps {
    onCodeHostDelete: (codeHost: EditableCodeHost) => void
}

/**
 * Renders edit UI for any supported code host type. (Github, Gitlab, ...)
 * Also performs edit, delete actions over opened code host connection
 */
export const CodeHostEdit: FC<CodeHostEditProps> = props => {
    const { onCodeHostDelete } = props
    const { codehostId } = useParams()

    const { data, loading, error, refetch } = useQuery<GetExternalServiceByIdResult, GetExternalServiceByIdVariables>(
        GET_CODE_HOST_BY_ID,
        {
            fetchPolicy: 'network-only',
            variables: { id: codehostId! },
        }
    )

    if (error && !loading) {
        return (
            <div>
                <ErrorAlert error={error} />
                <Button variant="secondary" outline={true} size="sm" onClick={() => refetch()}>
                    Try fetch again
                </Button>
            </div>
        )
    }

    if (!data || (!data && loading)) {
        return (
            <small className={styles.loadingState}>
                <LoadingSpinner /> Fetching connected code host...
            </small>
        )
    }

    if (data.node?.__typename !== 'ExternalService') {
        return (
            <Alert variant="warning">
                <H4>We either couldn't find code host</H4>
                Try to connect new code host instead <Link to="/setup/remote-repositories">here</Link>
            </Alert>
        )
    }

    return (
        <CodeHostEditView
            key={codehostId}
            codeHostId={codehostId!}
            codeHostKind={data.node.kind}
            displayName={data.node.displayName}
            configuration={data.node.config}
        >
            {state => (
                <footer className={styles.footer}>
                    <LoaderButton
                        type="submit"
                        variant="primary"
                        size="sm"
                        label={state.submitting ? 'Updating' : 'Update'}
                        alwaysShowLabel={true}
                        loading={state.submitting}
                        disabled={state.submitting}
                    />

                    <Button as={Link} size="sm" to="/setup/remote-repositories" variant="secondary">
                        Cancel
                    </Button>

                    <Button
                        variant="danger"
                        size="sm"
                        type="submit"
                        onClick={() => onCodeHostDelete(data.node as EditableCodeHost)}
                    >
                        Delete
                    </Button>
                </footer>
            )}
        </CodeHostEditView>
    )
}

interface CodeHostEditViewProps {
    codeHostId: string
    codeHostKind: ExternalServiceKind
    displayName: string
    configuration: string
    children: (state: CodeHostJSONFormState) => ReactNode
}

const CodeHostEditView: FC<CodeHostEditViewProps> = props => {
    const { codeHostId, codeHostKind, displayName, configuration, children } = props

    const navigate = useNavigate()
    const [updateRemoteCodeHost] = useMutation<UpdateRemoteCodeHostResult, UpdateRemoteCodeHostVariables>(
        UPDATE_CODE_HOST
    )

    const handleSubmit = async (input: AddExternalServiceInput): Promise<void> => {
        await updateRemoteCodeHost({
            variables: { input: { id: codeHostId, ...input } },
            refetchQueries: ['RepositoryStats', 'StatusMessages'],
        })

        navigate('/setup/remote-repositories')
        // TODO show notification UI that code host has been added successfully
    }

    const initialValues: CodeHostConnectFormFields = {
        displayName,
        config: configuration,
    }

    if (codeHostKind === ExternalServiceKind.GITHUB) {
        return (
            <GithubConnectView initialValues={initialValues} externalServiceId={codeHostId} onSubmit={handleSubmit}>
                {children}
            </GithubConnectView>
        )
    }

    return (
        <CodeHostJSONForm
            initialValues={initialValues}
            externalServiceOptions={defaultExternalServices[codeHostKind]}
            onSubmit={handleSubmit}
        >
            {children}
        </CodeHostJSONForm>
    )
}
