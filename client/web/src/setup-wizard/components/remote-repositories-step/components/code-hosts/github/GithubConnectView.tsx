import { FC, ReactNode, ReactElement, useCallback, useState, useMemo, ChangeEvent } from 'react'

import classNames from 'classnames'
import { parse as parseJSONC } from 'jsonc-parser'
import { noop } from 'lodash'

import { modify } from '@sourcegraph/common'
import { gql, useLazyQuery } from '@sourcegraph/http-client'
import {
    Tabs,
    Tab,
    TabList,
    TabPanel,
    TabPanels,
    Input,
    Checkbox,
    useLocalStorage,
    useField,
    useForm,
    FormInstance,
    getDefaultInputProps,
    useFieldAPI,
    useControlledField,
    ErrorAlert,
    FORM_ERROR,
    AsyncValidator,
} from '@sourcegraph/wildcard'

import { EXTERNAL_SERVICE_CHECK_CONNECTION_BY_ID } from '../../../../../../components/externalServices/backend'
import { codeHostExternalServices } from '../../../../../../components/externalServices/externalServices'
import {
    AddExternalServiceInput,
    ExternalServiceCheckConnectionByIdResult,
    ExternalServiceCheckConnectionByIdVariables,
    ExternalServiceKind,
    ValidateAccessTokenResult,
    ValidateAccessTokenVariables,
} from '../../../../../../graphql-operations'
import { CodeHostJSONFormContent, RadioGroupSection, CodeHostConnectFormFields, CodeHostJSONFormState } from '../common'

import { GithubOrganizationsPicker, GithubRepositoriesPicker } from './GithubEntityPickers'
import { getAccessTokenValue, getRepositoriesSettings } from './helpers'

import styles from './GithubConnectView.module.scss'

const DEFAULT_FORM_VALUES: CodeHostConnectFormFields = {
    displayName: codeHostExternalServices.github.defaultDisplayName,
    config: `
{
    "url": "https://github.com",
    "token": ""
}
`.trim(),
}

interface GithubConnectViewProps {
    initialValues?: CodeHostConnectFormFields
    externalServiceId?: string

    /**
     * Render props that is connected to form state, usually is used to render
     * form actions UI, like save, cancel, clear fields. Action layout is the same
     * for all variations of this form (create, edit UI) but content is different
     */
    children: (state: CodeHostJSONFormState) => ReactNode
    onSubmit: (input: AddExternalServiceInput) => Promise<any>
}

/**
 * GitHub's creation UI panel, it renders GitHub connection form UI and also handles
 * form values logic, like saving work-in-progress form values in local
 * storage
 */
export const GithubConnectView: FC<GithubConnectViewProps> = props => {
    const { initialValues, externalServiceId, children, onSubmit } = props
    const [localValues, setInitialValues] = useLocalStorage('github-connection-form', DEFAULT_FORM_VALUES)

    const handleSubmit = useCallback(
        async (values: CodeHostConnectFormFields): Promise<void> => {
            // Perform public API code host connection create action
            await onSubmit({
                kind: ExternalServiceKind.GITHUB,
                displayName: values.displayName,
                config: values.config,
            })

            // Reset initial values after successful connect action
            setInitialValues(DEFAULT_FORM_VALUES)
        },
        [setInitialValues, onSubmit]
    )

    return (
        <GithubConnectForm
            initialValues={initialValues ?? localValues}
            externalServiceId={externalServiceId}
            onChange={initialValues ? noop : setInitialValues}
            onSubmit={handleSubmit}
        >
            {children}
        </GithubConnectForm>
    )
}

enum GithubConnectFormTab {
    Form,
    JSONC,
}

interface GithubConnectFormProps {
    initialValues: CodeHostConnectFormFields
    externalServiceId?: string
    children: (state: CodeHostJSONFormState) => ReactNode
    onChange: (values: CodeHostConnectFormFields) => void
    onSubmit: (values: CodeHostConnectFormFields) => void
}

/**
 * It renders custom GitHub connect form that provides form UI and plain JSONC
 * configuration UI.
 */
export const GithubConnectForm: FC<GithubConnectFormProps> = props => {
    const { initialValues, externalServiceId, children, onChange, onSubmit } = props

    const [activeTab, setActiveTab] = useState(GithubConnectFormTab.Form)
    const form = useForm<CodeHostConnectFormFields>({
        initialValues,
        touched: !!externalServiceId,
        onSubmit,
        onChange: event => onChange(event.values),
    })

    const displayName = useField({
        formApi: form.formAPI,
        name: 'displayName',
        required: true,
    })

    const configuration = useField({
        formApi: form.formAPI,
        name: 'config',
    })

    return (
        <Tabs
            as="form"
            index={activeTab}
            lazy={true}
            size="medium"
            behavior="memoize"
            className={styles.form}
            onChange={setActiveTab}
            ref={form.ref}
            onSubmit={form.handleSubmit}
        >
            <TabList wrapperClassName={styles.tabList}>
                <Tab index={GithubConnectFormTab.Form} className={styles.tab}>
                    Settings
                </Tab>
                <Tab index={GithubConnectFormTab.JSONC} className={styles.tab}>
                    JSONC editor
                </Tab>
            </TabList>
            <TabPanels className={styles.tabPanels}>
                <TabPanel as="fieldset" tabIndex={-1} className={styles.formView}>
                    <GithubFormView
                        form={form}
                        displayNameField={displayName}
                        configurationField={configuration}
                        isTabActive={activeTab === GithubConnectFormTab.Form}
                        externalServiceId={externalServiceId}
                    />
                </TabPanel>
                <TabPanel as="fieldset" tabIndex={-1} className={styles.formView}>
                    <CodeHostJSONFormContent
                        displayNameField={displayName}
                        configurationField={configuration}
                        externalServiceOptions={codeHostExternalServices.github}
                    />
                </TabPanel>
                <>
                    {form.formAPI.submitErrors && (
                        <ErrorAlert className="w-100 mt-4" error={form.formAPI.submitErrors[FORM_ERROR]} />
                    )}
                </>
            </TabPanels>

            {children(form.formAPI)}
        </Tabs>
    )
}

interface GithubFormViewProps {
    isTabActive: boolean
    form: FormInstance<CodeHostConnectFormFields>
    displayNameField: useFieldAPI<string>
    configurationField: useFieldAPI<string>
    externalServiceId?: string
}

function GithubFormView(props: GithubFormViewProps): ReactElement {
    const { isTabActive, form, displayNameField, configurationField, externalServiceId } = props

    const accessTokenAsyncValidator = useAccessTokenValidator({ externalServiceId })
    const accessTokenField = useControlledField({
        value: getAccessTokenValue(configurationField.input.value),
        name: 'accessToken',
        submitted: form.formAPI.submitted,
        formTouched: form.formAPI.touched,
        validators: {
            sync: isTabActive ? syncAccessTokenValidator : undefined,
            async: isTabActive ? accessTokenAsyncValidator : undefined,
        },
        onChange: value => configurationField.input.onChange(modify(configurationField.input.value, ['token'], value)),
    })

    const { isAffiliatedRepositories, isOrgsRepositories, isSetRepositories, organizations, repositories } = useMemo(
        () => getRepositoriesSettings(configurationField.input.value),
        [configurationField.input.value]
    )

    const handleAffiliatedModeChange = (event: ChangeEvent<HTMLInputElement>): void => {
        const parsedConfiguration = parseJSONC(configurationField.input.value) as Record<string, any>
        const reposQuery: string[] =
            typeof parsedConfiguration === 'object' ? [...(parsedConfiguration.reposQuery ?? [])] : []

        const nextReposQuery = event.target.checked
            ? [...reposQuery, 'affiliated']
            : reposQuery.filter(token => token !== 'affiliated')

        configurationField.input.onChange(modify(configurationField.input.value, ['repositoryQuery'], nextReposQuery))
    }

    const handleOrganizationsModeChange = (event: ChangeEvent<HTMLInputElement>): void => {
        const nextConfiguration = event.target.checked
            ? modify(configurationField.input.value, ['orgs'], [])
            : modify(configurationField.input.value, ['orgs'], undefined)

        configurationField.input.onChange(nextConfiguration)
    }

    const handleRepositoriesModeChange = (event: ChangeEvent<HTMLInputElement>): void => {
        const nextConfiguration = event.target.checked
            ? modify(configurationField.input.value, ['repos'], [])
            : modify(configurationField.input.value, ['repos'], undefined)

        configurationField.input.onChange(nextConfiguration)
    }

    const handleOrganizationsChange = (organizations: string[]): void => {
        configurationField.input.onChange(modify(configurationField.input.value, ['orgs'], organizations))
    }

    const handleRepositoriesChange = (repositories: string[]): void => {
        configurationField.input.onChange(modify(configurationField.input.value, ['repos'], repositories))
    }

    // Fragment to avoid nesting since it's rendered within TabPanel fieldset
    return (
        <>
            <Input label="Display name" placeholder="Github (Personal)" {...getDefaultInputProps(displayNameField)} />

            <Input
                label="Access token"
                placeholder="Input your access token"
                message="Create a new access token on GitHub.com with repo or public_repo scope."
                {...getDefaultInputProps(accessTokenField)}
            />

            <section
                className={classNames(styles.repositoriesFields, {
                    [styles.repositoriesFieldsDisabled]: accessTokenField.meta.validState !== 'VALID',
                })}
            >
                <Checkbox
                    id="all-repos"
                    name="repositories"
                    label="Add all my repositories"
                    message="Will add all repositories affiliated with the token"
                    checked={isAffiliatedRepositories}
                    onChange={handleAffiliatedModeChange}
                />

                <RadioGroupSection
                    name="orgs"
                    value="orgs-repos"
                    checked={isOrgsRepositories}
                    labelId="orgs-repos"
                    label="Add all repositories from selected organizations or users"
                    onChange={handleOrganizationsModeChange}
                >
                    <GithubOrganizationsPicker
                        externalServiceId={externalServiceId}
                        token={accessTokenField.input.value}
                        disabled={accessTokenField.meta.validState !== 'VALID'}
                        organizations={organizations}
                        onChange={handleOrganizationsChange}
                    />
                </RadioGroupSection>

                <RadioGroupSection
                    name="repositories"
                    value="repositories"
                    checked={isSetRepositories}
                    labelId="repos"
                    label="Add selected repositories"
                    onChange={handleRepositoriesModeChange}
                >
                    <GithubRepositoriesPicker
                        externalServiceId={externalServiceId}
                        token={accessTokenField.input.value}
                        disabled={accessTokenField.meta.validState !== 'VALID'}
                        repositories={repositories}
                        onChange={handleRepositoriesChange}
                    />
                </RadioGroupSection>
            </section>
        </>
    )
}

function syncAccessTokenValidator(value: string | undefined): string | undefined {
    if (!value || value.length === 0) {
        return 'Access token is a required field'
    }

    return
}

/**
 * At the moment we don't have a designated query to check access token
 * validity, but as a workaround for this we use externalServiceRepositories
 * query that depends on the access token, so if this query resolves without
 * any error this means that token is valid.
 */
const CHECK_TOKEN_VALIDATION = gql`
    query ValidateAccessToken($token: String!) {
        externalServiceRepositories(
            token: $token
            first: 1
            query: ""
            kind: GITHUB
            excludeRepos: []
            url: "https://github.com"
        ) {
            nodes {
                id
            }
        }
    }
`

interface useAccessTokenValidatorInput {
    externalServiceId?: string
}

function useAccessTokenValidator(input: useAccessTokenValidatorInput): AsyncValidator<string> {
    const { externalServiceId } = input

    const [checkNewAccessToken] = useLazyQuery<ValidateAccessTokenResult, ValidateAccessTokenVariables>(
        CHECK_TOKEN_VALIDATION,
        { fetchPolicy: 'network-only' }
    )

    const [checkExternalServiceConnection] = useLazyQuery<
        ExternalServiceCheckConnectionByIdResult,
        ExternalServiceCheckConnectionByIdVariables
    >(EXTERNAL_SERVICE_CHECK_CONNECTION_BY_ID, { fetchPolicy: 'network-only' })

    return useCallback<AsyncValidator<string>>(
        async (token: string | undefined) => {
            if (!token) {
                return
            }

            // If we are in edit and access token has "REDACTED" value (which is default value
            // that BE returns for connected code hosts we should run check connection by its id
            // instead of making any token validation calls
            if (token === 'REDACTED' && externalServiceId) {
                const { data } = await checkExternalServiceConnection({ variables: { id: externalServiceId } })
                const externalService = data?.node

                // TS check since get external service by id lives in node(id: ID!) query
                // it's possible (im theory and API schema) to get non-external service entity
                // we should check it to get along with generated ts types
                if (externalService?.__typename !== 'ExternalService') {
                    return
                }

                switch (externalService.checkConnection.__typename) {
                    // Everything is ok, code host successfully checked and connected
                    case 'ExternalServiceAvailable':
                        return
                    case 'ExternalServiceUnavailable':
                        return externalService.checkConnection.suspectedReason
                    case 'ExternalServiceAvailabilityUnknown':
                        return "Check your access token, we couldn't reach out to code host by the current token"
                }
            }

            const { error } = await checkNewAccessToken({ variables: { token } })

            if (error) {
                return "Check your access token, we couldn't reach out to code host by the current token"
            }

            return
        },
        [checkExternalServiceConnection, checkNewAccessToken, externalServiceId]
    )
}
